package fastcache

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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
