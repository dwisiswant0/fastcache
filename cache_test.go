package fastcache

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCacheSmall(t *testing.T) {
	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	if _, ok := c.Get("aaa"); ok {
		t.Fatalf("unexpected value found for non-existent key")
	}

	if err := c.Set("key", "value"); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	if v, ok := c.Get("key"); !ok || v != "value" {
		t.Fatalf("unexpected value obtained; got %q; want %q", v, "value")
	}
	if _, ok := c.Get(""); ok {
		t.Fatalf("unexpected value found for empty key")
	}
	if _, ok := c.Get("aaa"); ok {
		t.Fatalf("unexpected value found for non-existent key")
	}

	if err := c.Set("aaa", "bbb"); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	if v, ok := c.Get("aaa"); !ok || v != "bbb" {
		t.Fatalf("unexpected value obtained; got %q; want %q", v, "bbb")
	}

	c.Reset()
	if _, ok := c.Get("aaa"); ok {
		t.Fatalf("unexpected value found after reset")
	}

	k := "empty"
	if err := c.Set(k, ""); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	if v, ok := c.Get(k); !ok {
		t.Fatalf("cannot find empty entry for key %q", k)
	} else if v != "" {
		t.Fatalf("unexpected non-empty value obtained from empty entry: %q", v)
	}
	if !c.Has(k) {
		t.Fatalf("cannot find empty entry for key %q", k)
	}
	if c.Has("foobar") {
		t.Fatalf("non-existing entry found in the cache")
	}
}

func TestCacheStringBytes(t *testing.T) {
	c, err := New[string, []byte](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	key := "key"
	value := []byte("value")

	if err := c.Set(key, value); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	if v, ok := c.Get(key); !ok || string(v) != string(value) {
		t.Fatalf("unexpected value obtained; got %q; want %q", v, value)
	}
}

func TestCacheWrap(t *testing.T) {
	c, err := New[string, string](1000)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	calls := 5000

	for i := range calls {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		if err := c.Set(k, v); err != nil {
			t.Fatalf("Set error: %s", err)
		}
		vv, ok := c.Get(k)
		if !ok || vv != v {
			t.Fatalf("unexpected value for key %q; got %q; want %q", k, vv, v)
		}
	}

	// Some entries may have been evicted
	hits := 0
	for i := range calls {
		k := fmt.Sprintf("key %d", i)
		if _, ok := c.Get(k); ok {
			hits++
		}
	}

	var s Stats
	c.UpdateStats(&s)
	if s.SetCalls != uint64(calls) {
		t.Fatalf("unexpected number of setCalls; got %d; want %d", s.SetCalls, calls)
	}
	if s.EntriesCount == 0 {
		t.Fatalf("unexpected zero entries count")
	}
	if s.MaxEntries != 1000 {
		t.Fatalf("unexpected MaxEntries; got %d; want %d", s.MaxEntries, 1000)
	}
}

func TestCacheDel(t *testing.T) {
	c, err := New[string, string](1024)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	for i := range 100 {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		if err := c.Set(k, v); err != nil {
			t.Fatalf("Set error: %s", err)
		}
		vv, ok := c.Get(k)
		if !ok || vv != v {
			t.Fatalf("unexpected value for key %q; got %q; want %q", k, vv, v)
		}
		c.Delete(k)
		if _, ok := c.Get(k); ok {
			t.Fatalf("unexpected value found for deleted key %q", k)
		}
	}
}

func TestCacheGetOrSet(t *testing.T) {
	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	// First call should store and return loaded=false
	actual, loaded, err := c.GetOrSet("key1", "value1")
	if err != nil {
		t.Fatalf("GetOrSet error: %s", err)
	}
	if loaded {
		t.Fatal("expected loaded=false for new key")
	}
	if actual != "value1" {
		t.Fatalf("unexpected value; got %q; want %q", actual, "value1")
	}

	// Second call should return existing and loaded=true
	actual, loaded, err = c.GetOrSet("key1", "value2")
	if err != nil {
		t.Fatalf("GetOrSet error: %s", err)
	}
	if !loaded {
		t.Fatal("expected loaded=true for existing key")
	}
	if actual != "value1" {
		t.Fatalf("unexpected value; got %q; want %q (original)", actual, "value1")
	}

	// Verify value wasn't overwritten
	v, ok := c.Get("key1")
	if !ok || v != "value1" {
		t.Fatalf("value was overwritten; got %q; want %q", v, "value1")
	}

	// Test with different key
	actual, loaded, err = c.GetOrSet("key2", "value2")
	if err != nil {
		t.Fatalf("GetOrSet error: %s", err)
	}
	if loaded {
		t.Fatal("expected loaded=false for new key2")
	}
	if actual != "value2" {
		t.Fatalf("unexpected value for key2; got %q; want %q", actual, "value2")
	}
}

func TestCacheGetAndDelete(t *testing.T) {
	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	// GetAndDelete on non-existent key
	v, loaded := c.GetAndDelete("nokey")
	if loaded {
		t.Fatal("expected loaded=false for non-existent key")
	}
	if v != "" {
		t.Fatalf("expected zero value; got %q", v)
	}

	// Set a key, then GetAndDelete
	if err := c.Set("key1", "value1"); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	v, loaded = c.GetAndDelete("key1")
	if !loaded {
		t.Fatal("expected loaded=true for existing key")
	}
	if v != "value1" {
		t.Fatalf("unexpected value; got %q; want %q", v, "value1")
	}

	// Verify key is gone
	if _, ok := c.Get("key1"); ok {
		t.Fatal("key should be deleted after GetAndDelete")
	}

	// GetAndDelete again should return loaded=false
	_, loaded = c.GetAndDelete("key1")
	if loaded {
		t.Fatal("expected loaded=false after key was deleted")
	}
}

func TestCacheSetIfAbsent(t *testing.T) {
	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	// First SetIfAbsent should succeed
	stored, err := c.SetIfAbsent("key1", "value1")
	if err != nil {
		t.Fatalf("SetIfAbsent error: %s", err)
	}
	if !stored {
		t.Fatal("expected stored=true for new key")
	}

	// Verify the value was stored
	v, ok := c.Get("key1")
	if !ok || v != "value1" {
		t.Fatalf("unexpected value; got %q; want %q", v, "value1")
	}

	// Second SetIfAbsent should fail (key exists)
	stored, err = c.SetIfAbsent("key1", "value2")
	if err != nil {
		t.Fatalf("SetIfAbsent error: %s", err)
	}
	if stored {
		t.Fatal("expected stored=false for existing key")
	}

	// Value should remain unchanged
	v, ok = c.Get("key1")
	if !ok || v != "value1" {
		t.Fatalf("value was overwritten; got %q; want %q", v, "value1")
	}

	// After delete, SetIfAbsent should succeed again
	c.Delete("key1")
	stored, err = c.SetIfAbsent("key1", "value3")
	if err != nil {
		t.Fatalf("SetIfAbsent error: %s", err)
	}
	if !stored {
		t.Fatal("expected stored=true after key was deleted")
	}

	v, ok = c.Get("key1")
	if !ok || v != "value3" {
		t.Fatalf("unexpected value after re-set; got %q; want %q", v, "value3")
	}
}

func TestCacheSetGetSerial(t *testing.T) {
	itemsCount := 10000
	c, err := New[string, string](itemsCount * 2)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()
	if err := testCacheGetSet(c, itemsCount); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestCacheGetSetConcurrent(t *testing.T) {
	itemsCount := 10000
	const goroutines = 10
	c, err := New[string, string](itemsCount * goroutines * 2)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	ch := make(chan error, goroutines)
	for range goroutines {
		go func() {
			ch <- testCacheGetSet(c, itemsCount)
		}()
	}
	for range goroutines {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout")
		}
	}
}

func testCacheGetSet(c *Cache[string, string], itemsCount int) error {
	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		if err := c.Set(k, v); err != nil {
			return fmt.Errorf("cannot set %q: %w", k, err)
		}
		vv, ok := c.Get(k)
		if !ok || vv != v {
			return fmt.Errorf("unexpected value for key %q after insertion; got %q; want %q", k, vv, v)
		}
	}
	misses := 0
	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		vExpected := fmt.Sprintf("value %d", i)
		v, ok := c.Get(k)
		if !ok || v != vExpected {
			if !ok {
				misses++
			} else {
				return fmt.Errorf("unexpected value for key %q after all insertions; got %q; want %q", k, v, vExpected)
			}
		}
	}
	if misses >= itemsCount/100 {
		return fmt.Errorf("too many cache misses; got %d; want less than %d", misses, itemsCount/100)
	}
	return nil
}

func TestCacheResetUpdateStatsSetConcurrent(t *testing.T) {
	c, err := New[string, string](12334)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	stopCh := make(chan struct{})

	// run workers for cache reset
	var resettersWG sync.WaitGroup
	for range 10 {
		resettersWG.Add(1)
		go func() {
			defer resettersWG.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					c.Reset()
					runtime.Gosched()
				}
			}
		}()
	}

	// run workers for update cache stats
	var statsWG sync.WaitGroup
	for range 10 {
		statsWG.Add(1)
		go func() {
			defer statsWG.Done()
			var s Stats
			for {
				select {
				case <-stopCh:
					return
				default:
					c.UpdateStats(&s)
					runtime.Gosched()
				}
			}
		}()
	}

	// run workers for setting data to cache
	var settersWG sync.WaitGroup
	for range 10 {
		settersWG.Add(1)
		go func() {
			defer settersWG.Done()
			for j := range 100 {
				key := fmt.Sprintf("key_%d", j)
				value := fmt.Sprintf("value_%d", j)
				if err := c.Set(key, value); err != nil {
					t.Errorf("Set(%q) error: %s", key, err)
					return
				}
				runtime.Gosched()
			}
		}()
	}

	// wait for setters
	settersWG.Wait()
	close(stopCh)
	statsWG.Wait()
	resettersWG.Wait()
}

func TestCacheRange(t *testing.T) {
	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	// Add entries
	for i := range 50 {
		if err := c.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i)); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	// Count entries via All
	count := 0
	for range c.All() {
		count++
	}

	if count != 50 {
		t.Fatalf("unexpected count from All; got %d; want 50", count)
	}

	// Test early exit
	count = 0
	for range c.All() {
		count++
		if count >= 10 {
			break
		}
	}

	if count != 10 {
		t.Fatalf("unexpected count with early exit; got %d; want 10", count)
	}
}

func TestCacheKeys(t *testing.T) {
	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	// Add entries
	for i := range 50 {
		if err := c.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i)); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	// Collect all keys
	keys := make(map[string]bool)
	for k := range c.Keys() {
		keys[k] = true
	}

	if len(keys) != 50 {
		t.Fatalf("unexpected key count; got %d; want 50", len(keys))
	}

	// Verify expected keys exist
	for i := range 50 {
		if !keys[fmt.Sprintf("key%d", i)] {
			t.Fatalf("missing key: key%d", i)
		}
	}

	// Test early exit
	count := 0
	for range c.Keys() {
		count++
		if count >= 10 {
			break
		}
	}

	if count != 10 {
		t.Fatalf("unexpected count with early exit; got %d; want 10", count)
	}
}

func TestCacheValues(t *testing.T) {
	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	// Add entries
	for i := range 50 {
		if err := c.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i)); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	// Collect all values
	values := make(map[string]bool)
	for v := range c.Values() {
		values[v] = true
	}

	if len(values) != 50 {
		t.Fatalf("unexpected value count; got %d; want 50", len(values))
	}

	// Verify expected values exist
	for i := range 50 {
		if !values[fmt.Sprintf("value%d", i)] {
			t.Fatalf("missing value: value%d", i)
		}
	}

	// Test early exit
	count := 0
	for range c.Values() {
		count++
		if count >= 10 {
			break
		}
	}

	if count != 10 {
		t.Fatalf("unexpected count with early exit; got %d; want 10", count)
	}
}

func TestCacheLen(t *testing.T) {
	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	if c.Len() != 0 {
		t.Fatalf("unexpected len for empty cache; got %d; want 0", c.Len())
	}

	for i := range 50 {
		if err := c.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i)); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	if c.Len() != 50 {
		t.Fatalf("unexpected len; got %d; want 50", c.Len())
	}

	c.Reset()
	if c.Len() != 0 {
		t.Fatalf("unexpected len after reset; got %d; want 0", c.Len())
	}
}

func TestCacheStruct(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}

	c, err := New[int, User](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	u := User{ID: 1, Name: "Alice"}
	if err := c.Set(1, u); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	got, ok := c.Get(1)
	if !ok {
		t.Fatal("user not found")
	}
	if got.ID != u.ID || got.Name != u.Name {
		t.Fatalf("unexpected user; got %+v; want %+v", got, u)
	}
}

func TestCacheHandlesForcedShardCollisions(t *testing.T) {
	const maxEntries = 8

	c, err := New[string, string](maxEntries)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	c.hasher = func(string) uint64 { return 1 }
	defer c.Reset()

	for i := range maxEntries * 2 {
		key := fmt.Sprintf("key-%d", i)
		value := fmt.Sprintf("value-%d", i)
		if err := c.Set(key, value); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	if c.Len() != maxEntries {
		t.Fatalf("unexpected len after forced collisions; got %d; want %d", c.Len(), maxEntries)
	}

	for i := range maxEntries {
		key := fmt.Sprintf("key-%d", i)
		if _, ok := c.Get(key); ok {
			t.Fatalf("unexpected stale key after fifo eviction under forced collisions: %q", key)
		}
	}

	for i := range maxEntries {
		idx := i + maxEntries
		key := fmt.Sprintf("key-%d", idx)
		want := fmt.Sprintf("value-%d", idx)
		got, ok := c.Get(key)
		if !ok || got != want {
			t.Fatalf("unexpected value for forced-collision key %q; got (%q, %t); want (%q, true)", key, got, ok, want)
		}
	}

	stored, err := c.SetIfAbsent("key-12", "newer")
	if err != nil {
		t.Fatalf("SetIfAbsent error: %s", err)
	}
	if stored {
		t.Fatal("SetIfAbsent unexpectedly overwrote an existing forced-collision key")
	}

	actual, loaded, err := c.GetOrSet("key-12", "newer")
	if err != nil {
		t.Fatalf("GetOrSet error: %s", err)
	}
	if !loaded || actual != "value-12" {
		t.Fatalf("GetOrSet returned (%q, %t); want (%q, true)", actual, loaded, "value-12")
	}

	deleted, ok := c.GetAndDelete("key-12")
	if !ok || deleted != "value-12" {
		t.Fatalf("GetAndDelete returned (%q, %t); want (%q, true)", deleted, ok, "value-12")
	}
	if _, ok := c.Get("key-12"); ok {
		t.Fatal("forced-collision key still present after GetAndDelete")
	}

	stored, err = c.SetIfAbsent("key-12", "replacement")
	if err != nil {
		t.Fatalf("SetIfAbsent error: %s", err)
	}
	if !stored {
		t.Fatal("SetIfAbsent failed after deleting a forced-collision key")
	}
	if got, ok := c.Get("key-12"); !ok || got != "replacement" {
		t.Fatalf("unexpected replacement value; got (%q, %t); want (%q, true)", got, ok, "replacement")
	}
}

func TestCacheEnforcesGlobalCapacityAcrossShards(t *testing.T) {
	c, err := New[string, string](4)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	c.hasher = func(k string) uint64 {
		if k[0] == 'a' {
			return 1
		}

		return 2
	}
	defer c.Reset()

	for i := range 4 {
		key := fmt.Sprintf("a%d", i)
		if err := c.Set(key, key); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	if err := c.Set("b0", "b0"); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	if c.Len() != 4 {
		t.Fatalf("unexpected len after cross-shard insertion at capacity; got %d; want 4", c.Len())
	}
	if _, ok := c.Get("a0"); ok {
		t.Fatal("oldest key remained after cross-shard insertion at capacity")
	}

	for _, key := range []string{"a1", "a2", "a3", "b0"} {
		got, ok := c.Get(key)
		if !ok || got != key {
			t.Fatalf("unexpected value for key %q; got (%q, %t)", key, got, ok)
		}
	}
}

func TestNewReturnsErrorForInvalidMaxEntries(t *testing.T) {
	cache, err := New[string, string](0)
	if !errors.Is(err, ErrInvalidMaxEntries) {
		t.Fatalf("New returned error %v; want %v", err, ErrInvalidMaxEntries)
	}
	if cache != nil {
		t.Fatal("New returned non-nil cache for invalid maxEntries")
	}
}

func TestCacheSetReturnsErrorWhenEvictionFails(t *testing.T) {
	c, err := New[string, string](1)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	c.hasher = func(string) uint64 { return 1 }
	c.shards[1].entries[1] = []entry[string, string]{{Key: "stale", Value: "value"}}
	c.shards[1].entryCount = 1
	c.entryCount.Store(1)

	err = c.Set("fresh", "value")
	if !errors.Is(err, ErrEvictionFailed) {
		t.Fatalf("Set returned error %v; want %v", err, ErrEvictionFailed)
	}
	if _, ok := c.Get("fresh"); ok {
		t.Fatal("Set inserted a key despite eviction failure")
	}
	if got := c.Len(); got != 1 {
		t.Fatalf("unexpected len after eviction failure; got %d; want 1", got)
	}
}
