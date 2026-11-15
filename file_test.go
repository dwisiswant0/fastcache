package fastcache

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/minio/minlz"
)

func TestSaveLoadSmall(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, "TestSaveLoadSmall.fastcache")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	key := "foobar"
	value := "abcdef"
	if err := c.Set(key, value); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	if err := c.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile error: %s", err)
	}

	c1, err := LoadFromFile[string, string](filePath)
	if err != nil {
		t.Fatalf("LoadFromFile error: %s", err)
	}
	vv, ok := c1.Get(key)
	if !ok || vv != value {
		t.Fatalf("unexpected value obtained from cache; got %q; want %q", vv, value)
	}

	// Verify that key can be overwritten.
	newValue := "234fdfd"
	if err := c1.Set(key, newValue); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	vv, ok = c1.Get(key)
	if !ok || vv != newValue {
		t.Fatalf("unexpected new value obtained from cache; got %q; want %q", vv, newValue)
	}
}

var errSync = errors.New("sync failed")

// newSyncTestCache returns a cache holding the entry foo=bar.
func newSyncTestCache(t *testing.T) *Cache[string, string] {
	t.Helper()

	cache, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	t.Cleanup(cache.Reset)

	err = cache.Set("foo", "bar")
	if err != nil {
		t.Fatalf("Set error: %s", err)
	}

	return cache
}

// replaceSyncFile installs hook as syncFile until the test ends.
func replaceSyncFile(t *testing.T, hook func(*os.File) error) {
	t.Helper()

	orig := syncFile

	t.Cleanup(func() { syncFile = orig })

	syncFile = hook
}

func TestSaveToFileSyncsBeforeRename(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "snapshot.fastcache")
	cache := newSyncTestCache(t)

	var synced []string

	replaceSyncFile(t, func(file *os.File) error {
		synced = append(synced, file.Name())

		_, err := os.Stat(filePath)
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("filePath must not exist before sync; Stat error: %v", err)
		}

		if got := filepath.Dir(file.Name()); got != dir {
			t.Errorf("synced file dir = %q; want %q", got, dir)
		}

		info, err := file.Stat()
		if err != nil {
			t.Errorf("Stat error: %s", err)

			return nil
		}

		if info.Size() == 0 {
			t.Errorf("synced file must hold the snapshot; got empty file")
		}

		return nil
	})

	err := cache.SaveToFile(filePath)
	if err != nil {
		t.Fatalf("SaveToFile error: %s", err)
	}

	if len(synced) != 1 {
		t.Fatalf("sync calls = %d; want 1", len(synced))
	}

	loaded, err := LoadFromFile[string, string](filePath)
	if err != nil {
		t.Fatalf("LoadFromFile error: %s", err)
	}

	defer loaded.Reset()

	v, ok := loaded.Get("foo")
	if !ok || v != "bar" {
		t.Fatalf("Get(foo) = %q, %v; want bar, true", v, ok)
	}
}

func TestSaveToFileSyncError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "snapshot.fastcache")
	cache := newSyncTestCache(t)

	// A failed save must leave the previous snapshot in place.
	old := []byte("previous snapshot")

	err := os.WriteFile(filePath, old, 0o600)
	if err != nil {
		t.Fatalf("WriteFile error: %s", err)
	}

	replaceSyncFile(t, func(*os.File) error { return errSync })

	err = cache.SaveToFile(filePath)
	if !errors.Is(err, errSync) {
		t.Fatalf("SaveToFile error = %v; want it to wrap %v", err, errSync)
	}

	got, err := os.ReadFile(filePath)
	if err != nil || !bytes.Equal(got, old) {
		t.Fatalf("filePath = %q, %v; want the previous snapshot %q", got, err, old)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %s", err)
	}

	for _, e := range entries {
		if e.Name() != filepath.Base(filePath) {
			t.Errorf("unexpected leftover file %q", e.Name())
		}
	}
}

func TestLoadFileNotExist(t *testing.T) {
	c, err := LoadFromFile[string, string](`non-existing-file`)
	if err == nil {
		t.Fatalf("LoadFromFile must return error; got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadFromFile must return os.ErrNotExist; got: %s", err)
	}
	if c != nil {
		t.Fatalf("LoadFromFile must return nil cache")
	}
}

func TestSaveLoadFile(t *testing.T) {
	for _, concurrency := range []int{0, 1, 2, 4, 10} {
		t.Run(fmt.Sprintf("concurrency_%d", concurrency), func(t *testing.T) {
			testSaveLoadFile(t, concurrency)
		})
	}
}

func testSaveLoadFile(t *testing.T, concurrency int) {
	var s Stats
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, fmt.Sprintf("TestSaveLoadFile.%d.fastcache", concurrency))
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	const itemsCount = 10000
	c, err := New[string, string](itemsCount * 2)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		if err := c.Set(k, v); err != nil {
			t.Fatalf("Set error: %s", err)
		}
		vv, ok := c.Get(k)
		if !ok || v != vv {
			t.Fatalf("unexpected cache value for k=%q; got %q; want %q", k, vv, v)
		}
	}
	if concurrency == 1 {
		if err := c.SaveToFile(filePath); err != nil {
			t.Fatalf("SaveToFile error: %s", err)
		}
	} else {
		if err := c.SaveToFileConcurrent(filePath, concurrency); err != nil {
			t.Fatalf("SaveToFileConcurrent(%d) error: %s", concurrency, err)
		}
	}
	s.Reset()
	c.UpdateStats(&s)
	if s.EntriesCount != itemsCount {
		t.Fatalf("unexpected entriesCount; got %d; want %d", s.EntriesCount, itemsCount)
	}
	c.Reset()

	// Verify LoadFromFile
	c, err = LoadFromFile[string, string](filePath)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	s.Reset()
	c.UpdateStats(&s)
	if s.EntriesCount != itemsCount {
		t.Fatalf("unexpected entriesCount; got %d; want %d", s.EntriesCount, itemsCount)
	}
	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		vv, ok := c.Get(k)
		if !ok || v != vv {
			t.Fatalf("unexpected cache value for k=%q; got %q; want %q", k, vv, v)
		}
	}
	c.Reset()

	// Verify LoadFromFileOrNew
	c, err = LoadFromFileOrNew[string, string](filePath, itemsCount*2)
	if err != nil {
		t.Fatalf("LoadFromFileOrNew error: %s", err)
	}
	s.Reset()
	c.UpdateStats(&s)
	if s.EntriesCount != itemsCount {
		t.Fatalf("unexpected entriesCount; got %d; want %d", s.EntriesCount, itemsCount)
	}
	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		vv, ok := c.Get(k)
		if !ok || v != vv {
			t.Fatalf("unexpected cache value for k=%q; got %q; want %q", k, vv, v)
		}
	}
	c.Reset()
}

func TestSaveLoadStruct(t *testing.T) {
	type User struct {
		ID   int
		Name string
		Tags []string
	}

	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, "TestSaveLoadStruct.fastcache")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	c, err := New[int, User](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	users := []User{
		{ID: 1, Name: "Alice", Tags: []string{"admin", "user"}},
		{ID: 2, Name: "Bob", Tags: []string{"user"}},
		{ID: 3, Name: "Charlie", Tags: nil},
	}

	for _, u := range users {
		if err := c.Set(u.ID, u); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	if err := c.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile error: %s", err)
	}

	c2, err := LoadFromFile[int, User](filePath)
	if err != nil {
		t.Fatalf("LoadFromFile error: %s", err)
	}

	for _, u := range users {
		got, ok := c2.Get(u.ID)
		if !ok {
			t.Fatalf("user %d not found after load", u.ID)
		}
		if got.ID != u.ID || got.Name != u.Name {
			t.Fatalf("unexpected user; got %+v; want %+v", got, u)
		}
		if len(got.Tags) != len(u.Tags) {
			t.Fatalf("unexpected tags length; got %d; want %d", len(got.Tags), len(u.Tags))
		}
	}
}

func TestLoadFromFileOrNew_NonExistent(t *testing.T) {
	c, err := LoadFromFileOrNew[string, string]("non-existing-file", 100)
	if err != nil {
		t.Fatalf("LoadFromFileOrNew error: %s", err)
	}
	if c == nil {
		t.Fatal("LoadFromFileOrNew returned nil")
	}

	// Should be an empty cache
	if c.Len() != 0 {
		t.Fatalf("expected empty cache; got %d entries", c.Len())
	}

	// Should be usable
	if err := c.Set("key", "value"); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	if v, ok := c.Get("key"); !ok || v != "value" {
		t.Fatalf("unexpected value; got %q; want %q", v, "value")
	}
}

// saveKeysToFile saves a cache that holds k-0 to k-(n-1), with capacity n, to
// a new file and returns its path.
func saveKeysToFile(t *testing.T, n int) string {
	t.Helper()

	c, err := New[string, int](n)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	setKeys(t, c, "k", n)

	path := filepath.Join(t.TempDir(), "cache")
	if err := c.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile error: %s", err)
	}

	return path
}

func TestLoadFromFileOrNewUsesGivenMaxEntries(t *testing.T) {
	t.Parallel()

	path := saveKeysToFile(t, 1000)

	// A smaller cache keeps the newest entries.
	small, err := LoadFromFileOrNew[string, int](path, 10)
	if err != nil {
		t.Fatalf("LoadFromFileOrNew error: %s", err)
	}

	var stats Stats

	small.UpdateStats(&stats)

	if stats.MaxEntries != 10 || stats.Evictions != 990 {
		t.Fatalf("cache loaded with maxEntries 10 has MaxEntries %d and %d evictions; want 10 and 990",
			stats.MaxEntries, stats.Evictions)
	}

	want := make([]string, 10)
	for i := range want {
		want[i] = fmt.Sprintf("k-%d", 990+i)
	}

	if got := slices.Collect(small.KeysFIFO()); !slices.Equal(got, want) {
		t.Fatalf("cache loaded with maxEntries 10 holds %v, oldest first; want %v", got, want)
	}

	// A larger cache holds every saved entry and has room for more.
	large, err := LoadFromFileOrNew[string, int](path, 2000)
	if err != nil {
		t.Fatalf("LoadFromFileOrNew error: %s", err)
	}

	stats.Reset()
	large.UpdateStats(&stats)

	if stats.MaxEntries != 2000 || stats.EntriesCount != 1000 {
		t.Fatalf("cache loaded with maxEntries 2000 has MaxEntries %d and %d entries; want 2000 and 1000",
			stats.MaxEntries, stats.EntriesCount)
	}
}

func TestLoadFromFileOrNewRejectsInvalidMaxEntries(t *testing.T) {
	t.Parallel()

	path := saveKeysToFile(t, 10)

	if _, err := LoadFromFileOrNew[string, int](path, 0); !errors.Is(err, ErrInvalidMaxEntries) {
		t.Fatalf("LoadFromFileOrNew(path, 0) error %v; want %v", err, ErrInvalidMaxEntries)
	}
}

func TestLoadFromFileOrNewReturnsEmptyCacheForTruncatedFile(t *testing.T) {
	t.Parallel()

	// Loading fails after it has inserted two entries.
	path := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(path, truncatedData(t, 2), 0o600); err != nil {
		t.Fatalf("WriteFile error: %s", err)
	}

	c, err := LoadFromFileOrNew[string, int](path, 50)
	if err != nil {
		t.Fatalf("LoadFromFileOrNew error: %s", err)
	}

	var stats Stats

	c.UpdateStats(&stats)

	if want := (Stats{MaxEntries: 50}); stats != want {
		t.Fatalf("cache loaded from a truncated file has stats %+v; want %+v", stats, want)
	}
}

func TestSaveToLoadFrom(t *testing.T) {
	const itemsCount = 1000
	c, err := New[string, string](itemsCount * 2)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		if err := c.Set(k, v); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	var buf bytes.Buffer
	if err := c.SaveTo(&buf); err != nil {
		t.Fatalf("SaveTo error: %s", err)
	}

	c2, err := LoadFrom[string, string](&buf)
	if err != nil {
		t.Fatalf("LoadFrom error: %s", err)
	}

	if c2.Len() != itemsCount {
		t.Fatalf("unexpected length; got %d; want %d", c2.Len(), itemsCount)
	}

	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		vv, ok := c2.Get(k)
		if !ok || v != vv {
			t.Fatalf("unexpected cache value for k=%q; got %q; want %q", k, vv, v)
		}
	}
}

func TestSaveToLoadFromKeepsEvictionOrder(t *testing.T) {
	t.Parallel()

	const n = 1000

	c, err := New[string, int](n)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	for i := range n {
		if err := c.Set(fmt.Sprintf("k%d", i), i); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	// Re-adding k0 makes it the newest entry, so it must be evicted last.
	c.Delete("k0")

	if err := c.Set("k0", 0); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	var buf bytes.Buffer
	if err := c.SaveTo(&buf); err != nil {
		t.Fatalf("SaveTo error: %s", err)
	}

	loaded, err := LoadFrom[string, int](&buf)
	if err != nil {
		t.Fatalf("LoadFrom error: %s", err)
	}

	// Eviction order is k1, k2, ..., k999, then k0.
	for i := range n {
		if err := loaded.Set(fmt.Sprintf("new%d", i), i); err != nil {
			t.Fatalf("Set error: %s", err)
		}

		if oldest := fmt.Sprintf("k%d", (i+1)%n); loaded.Has(oldest) {
			t.Fatalf("Set %d on the loaded cache did not evict %q", i, oldest)
		}
	}
}

func TestSaveToLoadFrom_Struct(t *testing.T) {
	type User struct {
		ID   int
		Name string
		Tags []string
	}

	c, err := New[int, User](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	users := []User{
		{ID: 1, Name: "Alice", Tags: []string{"admin", "user"}},
		{ID: 2, Name: "Bob", Tags: []string{"user"}},
		{ID: 3, Name: "Charlie", Tags: nil},
	}

	for _, u := range users {
		if err := c.Set(u.ID, u); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	var buf bytes.Buffer
	if err := c.SaveTo(&buf); err != nil {
		t.Fatalf("SaveTo error: %s", err)
	}

	c2, err := LoadFrom[int, User](&buf)
	if err != nil {
		t.Fatalf("LoadFrom error: %s", err)
	}

	for _, u := range users {
		got, ok := c2.Get(u.ID)
		if !ok {
			t.Fatalf("user %d not found after load", u.ID)
		}
		if got.ID != u.ID || got.Name != u.Name {
			t.Fatalf("unexpected user; got %+v; want %+v", got, u)
		}
		if len(got.Tags) != len(u.Tags) {
			t.Fatalf("unexpected tags length; got %d; want %d", len(got.Tags), len(u.Tags))
		}
	}
}

func TestLoadFrom_EmptyReader(t *testing.T) {
	var buf bytes.Buffer
	_, err := LoadFrom[string, string](&buf)
	if err == nil {
		t.Fatal("LoadFrom must return error for empty reader")
	}
}

func TestSaveToFuncLoadFromFuncFilterAndConvert(t *testing.T) {
	t.Parallel()

	// SaveTo rejects item, since gob does not encode unexported fields.
	type item struct {
		data    string
		private bool
	}

	type saved struct{ Data string }

	c, err := New[string, *item](10)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	for i := range 10 {
		if err := c.Set(fmt.Sprintf("k%d", i), &item{data: fmt.Sprintf("v%d", i), private: i%2 == 1}); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	var buf bytes.Buffer

	err = SaveToFunc(c, &buf, func(_ string, v *item) (saved, bool) {
		return saved{Data: v.data}, !v.private
	})
	if err != nil {
		t.Fatalf("SaveToFunc error: %s", err)
	}

	// Loading oldest first leaves the newest saved entries in a smaller cache.
	next, err := New[string, *item](3)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	err = LoadFromFunc(&buf, func(k string, s saved) error {
		return next.Set(k, &item{data: s.Data})
	})
	if err != nil {
		t.Fatalf("LoadFromFunc error: %s", err)
	}

	var got []string
	for k, v := range next.AllFIFO() {
		got = append(got, k+"="+v.data)
	}

	if want := []string{"k4=v4", "k6=v6", "k8=v8"}; !slices.Equal(got, want) {
		t.Fatalf("loaded cache holds %v; want %v", got, want)
	}
}

func TestSaveToFuncDataLoadsWithLoadFrom(t *testing.T) {
	t.Parallel()

	c, err := New[string, int](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	setKeys(t, c, "k", 5)

	var buf bytes.Buffer

	err = SaveToFunc(c, &buf, func(_ string, v int) (string, bool) {
		return strconv.Itoa(v), true
	})
	if err != nil {
		t.Fatalf("SaveToFunc error: %s", err)
	}

	loaded, err := LoadFrom[string, string](&buf)
	if err != nil {
		t.Fatalf("LoadFrom error: %s", err)
	}

	var stats Stats

	loaded.UpdateStats(&stats)

	if stats.MaxEntries != 100 {
		t.Fatalf("loaded cache has MaxEntries %d; want 100", stats.MaxEntries)
	}

	got := slices.Collect(loaded.Values())
	slices.Sort(got)

	if want := []string{"0", "1", "2", "3", "4"}; !slices.Equal(got, want) {
		t.Fatalf("loaded values %v; want %v", got, want)
	}
}

func TestLoadFromFuncReadsSaveToDataOldestFirst(t *testing.T) {
	t.Parallel()

	c, err := New[string, int](10)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	setKeys(t, c, "k", 3)

	// Setting k-0 again after deleting it makes it the newest entry.
	c.Delete("k-0")

	if err := c.Set("k-0", 0); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	var buf bytes.Buffer
	if err := c.SaveTo(&buf); err != nil {
		t.Fatalf("SaveTo error: %s", err)
	}

	var got []string

	err = LoadFromFunc(&buf, func(k string, v int) error {
		got = append(got, fmt.Sprintf("%s=%d", k, v))

		return nil
	})
	if err != nil {
		t.Fatalf("LoadFromFunc error: %s", err)
	}

	if want := []string{"k-1=1", "k-2=2", "k-0=0"}; !slices.Equal(got, want) {
		t.Fatalf("LoadFromFunc loaded %v; want %v", got, want)
	}
}

func TestLoadFromFuncStopsAtLoadError(t *testing.T) {
	t.Parallel()

	c, err := New[string, int](10)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	setKeys(t, c, "k", 5)

	var buf bytes.Buffer
	if err := c.SaveTo(&buf); err != nil {
		t.Fatalf("SaveTo error: %s", err)
	}

	errStop := errors.New("stop")
	calls := 0

	err = LoadFromFunc(&buf, func(string, int) error {
		calls++
		if calls == 2 {
			return errStop
		}

		return nil
	})
	if err != errStop { //nolint:errorlint // LoadFromFunc must return the error of load itself.
		t.Fatalf("LoadFromFunc error %v; want %v", err, errStop)
	}

	if calls != 2 {
		t.Fatalf("LoadFromFunc called load %d times after it failed on call 2", calls)
	}
}

// truncatedData returns data saved from a cache of capacity 100 whose entry
// count promises one more entry than follows it: k-0 to k-(n-1), with values
// 0 to n-1.
func truncatedData(t *testing.T, n int) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := minlz.NewWriter(&buf)
	enc := gob.NewEncoder(zw)

	write := func(v any) {
		if err := enc.Encode(v); err != nil {
			t.Fatalf("Encode error: %s", err)
		}
	}

	write(100)
	write(n + 1)

	for i := range n {
		write(&entry[string, int]{Key: fmt.Sprintf("k-%d", i), Value: i})
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("Close error: %s", err)
	}

	return buf.Bytes()
}

func TestLoadFromFuncReturnsErrorForTruncatedData(t *testing.T) {
	t.Parallel()

	var got []string

	err := LoadFromFunc(bytes.NewReader(truncatedData(t, 2)), func(k string, _ int) error {
		got = append(got, k)

		return nil
	})
	if err == nil {
		t.Fatal("LoadFromFunc must return error for truncated data")
	}

	if want := []string{"k-0", "k-1"}; !slices.Equal(got, want) {
		t.Fatalf("LoadFromFunc passed %v to load before it failed; want %v", got, want)
	}
}

func TestLoadFromFunc_EmptyReader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := LoadFromFunc(&buf, func(string, int) error { return nil })
	if err == nil {
		t.Fatal("LoadFromFunc must return error for empty reader")
	}
}

func TestSaveToFuncRejectsUnsupportedSavedType(t *testing.T) {
	t.Parallel()

	c, err := New[string, int](10)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	setKeys(t, c, "k", 1)

	type unexported struct{ n int }

	var buf bytes.Buffer

	err = SaveToFunc(c, &buf, func(_ string, v int) (unexported, bool) {
		return unexported{n: v}, true
	})
	if !errors.Is(err, errUnsupportedType) {
		t.Fatalf("SaveToFunc error %v; want %v", err, errUnsupportedType)
	}

	if buf.Len() != 0 {
		t.Fatalf("SaveToFunc wrote %d bytes before failing", buf.Len())
	}
}

// selfEncoder holds only unexported state but encodes itself, so gob keeps it.
type selfEncoder struct{ n byte }

func (s selfEncoder) GobEncode() ([]byte, error) {
	return []byte{s.n}, nil
}

func (s *selfEncoder) GobDecode(b []byte) error {
	s.n = b[0]

	return nil
}

// ptrEncoder implements [encoding.BinaryMarshaler] on the pointer receiver.
type ptrEncoder struct{ n byte }

func (p *ptrEncoder) MarshalBinary() ([]byte, error) {
	return []byte{p.n}, nil
}

func (p *ptrEncoder) UnmarshalBinary(b []byte) error {
	p.n = b[0]

	return nil
}

// hookFunc encodes itself, but gob still drops it as a struct field, as it
// does every chan or func field.
type hookFunc func() int

func (hookFunc) GobEncode() ([]byte, error) { return []byte{1}, nil }

func (*hookFunc) GobDecode([]byte) error { return nil }

// ptrCycleA and ptrCycleB point at each other.
type (
	ptrCycleA *ptrCycleB
	ptrCycleB *ptrCycleA
)

type hiddenFields struct {
	a int
}

type node struct {
	V    int
	Next *node
}

type badNode struct {
	v    int
	Next *badNode
}

// roundTrip stores key and value in a cache, saves it to a buffer and loads
// it back. It returns the save error, if any. On success it verifies that
// the loaded cache holds the same value.
func roundTrip[K comparable, V any](t *testing.T, key K, value V) error {
	t.Helper()

	cache, err := New[K, V](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	if err := cache.Set(key, value); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	var buf bytes.Buffer
	if err := cache.SaveTo(&buf); err != nil {
		if buf.Len() != 0 {
			t.Fatalf("SaveTo wrote %d bytes before returning error", buf.Len())
		}

		return err
	}

	loaded, err := LoadFrom[K, V](&buf)
	if err != nil {
		t.Fatalf("LoadFrom error: %s", err)
	}

	got, ok := loaded.Get(key)
	if !ok || !reflect.DeepEqual(got, value) {
		t.Fatalf("unexpected value after load; got %+v (found=%v); want %+v", got, ok, value)
	}

	return nil
}

func TestSaveToUnsupportedType(t *testing.T) {
	type point struct{ x, y int }

	type counter struct {
		Name  string
		count int
	}

	type withFunc struct {
		Name string
		Fn   func()
	}

	type withChan struct {
		Ch chan int
	}

	type (
		nested   struct{ In hiddenFields }
		inSlice  struct{ Items []hiddenFields }
		inArray  struct{ Items [2]hiddenFields }
		inMap    struct{ M map[string]hiddenFields }
		inMapKey struct{ M map[hiddenFields]string }
		inPtr    struct{ P *hiddenFields }
		inPtrPtr struct{ P **hiddenFields }
	)

	type key struct {
		ID  int
		seq int
	}

	type blank struct {
		A int
		_ [0]func()
		B string
	}

	type selfEncoding struct {
		At time.Time
		S  selfEncoder
		P  *ptrEncoder
	}

	// Zero-size fields hold no data, so gob dropping them loses nothing.
	type (
		noCopy     struct{}
		withNoCopy struct {
			Name   string
			noCopy noCopy
		}
		zeroArray struct {
			A   int
			Arr [0]counter
		}
	)

	stamp := time.Date(2026, time.January, 12, 10, 32, 4, 0, time.UTC)

	tests := []struct {
		name string
		run  func(t *testing.T) error
		// want lists substrings of the error; nil means the save must succeed.
		want []string
	}{
		{
			name: "no exported fields",
			run:  func(t *testing.T) error { return roundTrip(t, 1, point{1, 2}) },
			want: []string{"field x of", "fastcache.point"},
		},
		{
			name: "unexported field",
			run:  func(t *testing.T) error { return roundTrip(t, 1, counter{"a", 3}) },
			want: []string{"unexported field count of fastcache.counter", "value type fastcache.counter"},
		},
		{
			name: "exported func field",
			run:  func(t *testing.T) error { return roundTrip(t, 1, withFunc{Name: "a"}) },
			want: []string{"field Fn of", "fastcache.withFunc"},
		},
		{
			name: "exported chan field",
			run:  func(t *testing.T) error { return roundTrip(t, 1, withChan{}) },
			want: []string{"field Ch of", "fastcache.withChan"},
		},
		{
			name: "nested struct",
			run:  func(t *testing.T) error { return roundTrip(t, 1, nested{hiddenFields{1}}) },
			want: []string{"field a of fastcache.hiddenFields", "field In of fastcache.nested"},
		},
		{
			name: "slice elem",
			run:  func(t *testing.T) error { return roundTrip(t, 1, inSlice{[]hiddenFields{{1}}}) },
			want: []string{"field a of", "fastcache.hiddenFields"},
		},
		{
			name: "array elem",
			run:  func(t *testing.T) error { return roundTrip(t, 1, inArray{}) },
			want: []string{"field a of", "fastcache.hiddenFields"},
		},
		{
			name: "map elem",
			run: func(t *testing.T) error {
				return roundTrip(t, 1, inMap{map[string]hiddenFields{"k": {1}}})
			},
			want: []string{"field a of", "fastcache.hiddenFields"},
		},
		{
			name: "map key",
			run: func(t *testing.T) error {
				return roundTrip(t, 1, inMapKey{map[hiddenFields]string{{1}: "v"}})
			},
			want: []string{"field a of", "fastcache.hiddenFields"},
		},
		{
			name: "pointer elem",
			run:  func(t *testing.T) error { return roundTrip(t, 1, inPtr{&hiddenFields{1}}) },
			want: []string{"field a of", "fastcache.hiddenFields"},
		},
		{
			name: "pointer to pointer elem",
			run:  func(t *testing.T) error { return roundTrip(t, 1, inPtrPtr{}) },
			want: []string{"field a of", "fastcache.hiddenFields"},
		},
		{
			name: "top-level pointer value",
			run:  func(t *testing.T) error { return roundTrip(t, 1, &hiddenFields{1}) },
			want: []string{"field a of", "fastcache.hiddenFields"},
		},
		{
			name: "key with unexported field",
			run:  func(t *testing.T) error { return roundTrip(t, key{1, 2}, "v") },
			want: []string{"unexported field seq of fastcache.key", "key type fastcache.key"},
		},
		{
			name: "recursive type with unexported field",
			run:  func(t *testing.T) error { return roundTrip(t, 1, badNode{1, nil}) },
			want: []string{"field v of", "fastcache.badNode"},
		},
		{
			name: "recursive type",
			run:  func(t *testing.T) error { return roundTrip(t, 1, node{1, &node{V: 2}}) },
		},
		{
			name: "time.Time",
			run:  func(t *testing.T) error { return roundTrip(t, 1, stamp) },
		},
		{
			name: "time.Time key",
			run:  func(t *testing.T) error { return roundTrip(t, stamp, 1) },
		},
		{
			name: "self-encoding types",
			run: func(t *testing.T) error {
				return roundTrip(t, 1, selfEncoding{stamp, selfEncoder{3}, &ptrEncoder{4}})
			},
		},
		{
			name: "self-encoding value reached through pointer",
			run:  func(t *testing.T) error { return roundTrip(t, 1, []*selfEncoder{{5}}) },
		},
		{
			name: "chan key",
			run:  func(t *testing.T) error { return roundTrip(t, make(chan int), 1) },
			want: []string{"chan int", "key type chan int"},
		},
		{
			name: "func value",
			run:  func(t *testing.T) error { return roundTrip(t, 1, func() {}) },
			want: []string{"func()", "value type func()"},
		},
		{
			name: "self-encoding func value",
			run:  func(t *testing.T) error { return roundTrip(t, 1, hookFunc(func() int { return 1 })) },
			want: []string{"gob does not encode fastcache.hookFunc", "value type fastcache.hookFunc"},
		},
		{
			name: "self-encoding func field",
			run: func(t *testing.T) error {
				return roundTrip(t, 1, struct {
					Name string
					H    *hookFunc
				}{Name: "a", H: new(hookFunc)})
			},
			want: []string{"gob does not encode *fastcache.hookFunc", "field H of"},
		},
		{
			name: "self-encoding func slice elem",
			run: func(t *testing.T) error {
				cache, err := New[int, []hookFunc](1)
				if err != nil {
					t.Fatalf("New error: %s", err)
				}

				if err := cache.Set(1, []hookFunc{nil}); err != nil {
					t.Fatalf("Set error: %s", err)
				}

				return cache.SaveTo(io.Discard)
			},
		},
		{
			name: "unexported zero-size field",
			run:  func(t *testing.T) error { return roundTrip(t, 1, withNoCopy{Name: "a", noCopy: noCopy{}}) },
		},
		{
			name: "zero-length array of unsupported type",
			run:  func(t *testing.T) error { return roundTrip(t, 1, zeroArray{A: 1, Arr: [0]counter{}}) },
		},
		{
			name: "blank field",
			run:  func(t *testing.T) error { return roundTrip(t, 1, blank{A: 1, B: "b"}) },
		},
		{
			name: "plain types",
			run:  func(t *testing.T) error { return roundTrip(t, "k", map[string][]int{"a": {1}}) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(t)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("SaveTo error: %s", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("SaveTo must return error; got nil")
			}

			if !errors.Is(err, errUnsupportedType) {
				t.Fatalf("unexpected error; got %v; want it to wrap errUnsupportedType", err)
			}

			for _, s := range tt.want {
				if !strings.Contains(err.Error(), s) {
					t.Fatalf("error %q does not mention %q", err, s)
				}
			}
		})
	}
}

func TestSaveToRecursivePointerTypeReturns(t *testing.T) {
	t.Parallel()

	type recPtr *recPtr

	saves := map[string]func() error{
		"self":         newSaver[recPtr](),
		"mutual":       newSaver[ptrCycleA](),
		"mutual field": newSaver[struct{ P ptrCycleA }](),
	}

	for name, save := range saves {
		done := make(chan error, 1)

		go func() { done <- save() }()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: SaveTo error: %s", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: SaveTo did not return for a recursive pointer type", name)
		}
	}
}

// newSaver returns a function that saves an empty cache with values of type V.
func newSaver[V any]() func() error {
	return func() error {
		cache, err := New[int, V](1)
		if err != nil {
			return err
		}

		return cache.SaveTo(io.Discard)
	}
}

func TestSaveToFileUnsupportedType(t *testing.T) {
	type counter struct {
		Name  string
		count int
	}

	cache, err := New[int, counter](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	if err := cache.Set(1, counter{"a", 3}); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "unsupported.fastcache")

	err = cache.SaveToFile(filePath)
	if err == nil {
		t.Fatalf("SaveToFile must return error; got nil")
	}

	for _, s := range []string{"field count of", "fastcache.counter"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("error %q does not mention %q", err, s)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %s", err)
	}

	if len(entries) != 0 {
		t.Fatalf("SaveToFile left %d files behind", len(entries))
	}
}
