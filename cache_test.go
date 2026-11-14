package fastcache

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
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

func TestCacheEvictsReAddedKeyInNewInsertionOrder(t *testing.T) {
	t.Parallel()

	removers := []struct {
		name   string
		remove func(c *Cache[string, int], k string)
	}{
		{"Delete", func(c *Cache[string, int], k string) { c.Delete(k) }},
		{"GetAndDelete", func(c *Cache[string, int], k string) { c.GetAndDelete(k) }},
	}

	for _, r := range removers {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			c, err := New[string, int](3)
			if err != nil {
				t.Fatalf("New error: %s", err)
			}
			defer c.Reset()

			for i, k := range []string{"a", "b", "c"} {
				if err := c.Set(k, i); err != nil {
					t.Fatalf("Set(%q) error: %s", k, err)
				}
			}

			r.remove(c, "a")

			if err := c.Set("a", 3); err != nil {
				t.Fatalf("Set(%q) error: %s", "a", err)
			}

			// Insertion order is now b, c, a, so each new key evicts them in
			// that order.
			for _, step := range []struct{ set, evicted string }{
				{"d", "b"},
				{"e", "c"},
				{"f", "a"},
			} {
				if err := c.Set(step.set, 0); err != nil {
					t.Fatalf("Set(%q) error: %s", step.set, err)
				}

				if c.Has(step.evicted) {
					t.Fatalf("Set(%q) did not evict %q", step.set, step.evicted)
				}

				if c.Len() != 3 {
					t.Fatalf("unexpected len after Set(%q); got %d; want 3", step.set, c.Len())
				}
			}
		})
	}
}

func TestCacheDeleteChurnKeepsOrderLogBounded(t *testing.T) {
	t.Parallel()

	keyFuncs := []struct {
		name string
		key  func(i int) string
	}{
		{"distinct keys", func(i int) string { return fmt.Sprintf("key-%d", i) }},
		{"same key", func(int) string { return "k" }},
	}

	removers := []struct {
		name   string
		remove func(c *Cache[string, int], k string)
	}{
		{"Delete", func(c *Cache[string, int], k string) { c.Delete(k) }},
		{"GetAndDelete", func(c *Cache[string, int], k string) { c.GetAndDelete(k) }},
	}

	for _, kf := range keyFuncs {
		for _, r := range removers {
			t.Run(kf.name+"/"+r.name, func(t *testing.T) {
				t.Parallel()

				testDeleteChurnKeepsOrderLogBounded(t, kf.key, r.remove)
			})
		}
	}
}

func testDeleteChurnKeepsOrderLogBounded(t *testing.T, key func(int) string, remove func(*Cache[string, int], string)) {
	t.Helper()

	c, err := New[string, int](1000)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	for i := range 100_000 {
		k := key(i)
		if err := c.Set(k, i); err != nil {
			t.Fatalf("Set(%q) error: %s", k, err)
		}

		remove(c, k)
	}

	// With no live entries, the log may hold only the dead slots that have
	// not yet reached the compaction threshold, plus the last one.
	c.orderMu.Lock()
	pending := c.order.len
	c.orderMu.Unlock()

	if pending > minCompactSlots+1 {
		t.Fatalf("order log holds %d slots for an empty cache; want at most %d", pending, minCompactSlots+1)
	}
}

func TestCacheEvictsInInsertionOrderAfterDeleteChurn(t *testing.T) {
	t.Parallel()

	hashers := []struct {
		name   string
		hasher func(string) uint64
	}{
		{"rapidhash", nil},
		{"one bucket", func(string) uint64 { return 1 }},
	}

	for _, h := range hashers {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()

			testEvictsInInsertionOrderAfterDeleteChurn(t, h.hasher)
		})
	}
}

func testEvictsInInsertionOrderAfterDeleteChurn(t *testing.T, hasher func(string) uint64) {
	t.Helper()

	const live = 1000

	c, err := New[string, int](2 * live)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	if hasher != nil {
		c.hasher = hasher
	}

	setKeys(t, c, "live", live)

	for i := range 10 * minCompactSlots {
		k := fmt.Sprintf("churn-%d", i)
		if err := c.Set(k, i); err != nil {
			t.Fatalf("Set(%q) error: %s", k, err)
		}

		c.Delete(k)
	}

	setKeys(t, c, "fill", live)

	for i := range live {
		over, oldest := fmt.Sprintf("over-%d", i), fmt.Sprintf("live-%d", i)
		if err := c.Set(over, i); err != nil {
			t.Fatalf("Set(%q) error: %s", over, err)
		}

		if c.Has(oldest) {
			t.Fatalf("Set(%q) did not evict %q", over, oldest)
		}

		if next := fmt.Sprintf("live-%d", i+1); i+1 < live && !c.Has(next) {
			t.Fatalf("Set(%q) evicted %q out of insertion order", over, next)
		}
	}
}

func setKeys(t *testing.T, c *Cache[string, int], prefix string, n int) {
	t.Helper()

	for i := range n {
		k := fmt.Sprintf("%s-%d", prefix, i)
		if err := c.Set(k, i); err != nil {
			t.Fatalf("Set(%q) error: %s", k, err)
		}
	}
}

// Not parallel: TotalAlloc also counts allocations made by other tests.
func TestNewAllocatesIndependentlyOfMaxEntries(t *testing.T) {
	allocated := func(maxEntries int) uint64 {
		var before, after runtime.MemStats

		runtime.ReadMemStats(&before)
		c, err := New[string, []byte](maxEntries)

		runtime.ReadMemStats(&after)

		if err != nil {
			t.Fatalf("New(%d) error: %s", maxEntries, err)
		}

		runtime.KeepAlive(c)

		return after.TotalAlloc - before.TotalAlloc
	}

	small, large := allocated(1), allocated(1_000_000)
	if large > 2*small {
		t.Fatalf("New(1_000_000) allocated %d bytes; New(1) allocated %d", large, small)
	}
}

// Not parallel: HeapAlloc also counts memory held by other tests.
func TestCacheResetReleasesMemory(t *testing.T) {
	const n = 1 << 17

	heapAlloc := func() uint64 {
		var m runtime.MemStats

		runtime.GC()
		runtime.ReadMemStats(&m)

		return m.HeapAlloc
	}

	c, err := New[int, int](n)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	empty := heapAlloc()

	for i := range n {
		if err := c.Set(i, i); err != nil {
			t.Fatalf("Set(%d) error: %s", i, err)
		}
	}

	used := heapAlloc() - empty

	c.Reset()

	reset := heapAlloc()

	runtime.KeepAlive(c)

	// Keeping only the order log, at 24 bytes per entry, holds about a fifth
	// of used.
	if reset > empty+used/10 {
		t.Fatalf("after Reset the cache holds %d of the %d bytes its %d entries used", reset-empty, used, n)
	}
}

// TestCacheEvictionCostIndependentOfMaxEntries checks that evicting from a
// full cache never does work in proportion to the cache size. Such work,
// recurring in every window of evictions, takes hundreds of times the mean,
// while scheduling and GC pauses leave at least one window alone.
//
// Not parallel: other tests would add their own pauses to the timings.
func TestCacheEvictionCostIndependentOfMaxEntries(t *testing.T) {
	// The Go runtime reads the Windows monotonic clock from the interrupt
	// time, which advances only on timer ticks of 0.5 ms or more.
	if runtime.GOOS == "windows" {
		t.Skip("the monotonic clock is too coarse to time a single Set")
	}

	const (
		maxEntries = 1 << 18
		windows    = 16
		window     = 1 << 11
	)

	c, err := New[int, struct{}](maxEntries)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	// Fill the cache, then replace every entry once before timing. Shards
	// whose entry counts wander across a growth threshold in the first
	// turnover grow their tables, each once; that belongs to filling, not to
	// evicting.
	for i := range 2 * maxEntries {
		if err := c.Set(i, struct{}{}); err != nil {
			t.Fatalf("Set(%d) error: %s", i, err)
		}
	}

	var total time.Duration

	slowest := time.Duration(math.MaxInt64) // the fastest window's slowest Set
	next := 2 * maxEntries

	for range windows {
		var worst time.Duration

		for range window {
			start := time.Now()
			if err := c.Set(next, struct{}{}); err != nil {
				t.Fatalf("Set(%d) error: %s", next, err)
			}

			took := time.Since(start)
			total += took
			worst = max(worst, took)
			next++
		}

		slowest = min(slowest, worst)
	}

	if mean := total / (windows * window); slowest > 50*mean {
		t.Fatalf("every window of %d evictions had one taking %v or more; the mean is %v", window, slowest, mean)
	}
}

// TestSlotRingKeepsFIFOOrder mixes pushes, pops, and retains so that the ring
// wraps around before it grows or drops slots. The cache tests rarely get
// there: the ring wraps only after evictions, and grows after that only when
// deleted entries pile up.
func TestSlotRingKeepsFIFOOrder(t *testing.T) {
	t.Parallel()

	var (
		ring slotRing
		want []slot
		seq  uint64
	)

	keep := func(s slot) bool { return s.seq%3 != 0 }

	for round := range 300 {
		for range round%7 + 3 {
			seq++
			ring.push(slot{seq: seq})
			want = append(want, slot{seq: seq})
		}

		for range min(round%5, len(want)) {
			if got := ring.pop(); got != want[0] {
				t.Fatalf("round %d: pop returned seq %d; want %d", round, got.seq, want[0].seq)
			}

			want = want[1:]
		}

		if round%11 == 0 {
			ring.retain(keep)

			want = slices.DeleteFunc(want, func(s slot) bool { return !keep(s) })
		}

		if ring.len != len(want) {
			t.Fatalf("round %d: ring holds %d slots; want %d", round, ring.len, len(want))
		}

		for i, s := range want {
			if got := ring.buf[ring.index(i)]; got != s {
				t.Fatalf("round %d: slot %d has seq %d; want %d", round, i, got.seq, s.seq)
			}
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
	c.shards[1].insertLocked(&entry[string, string]{Key: "stale", Value: "value", hash: 1})
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

// TestCacheSetSucceedsWhileDeletesEmptyIt checks that Set does not fail when
// a Delete removes the entry that Set was about to evict, which leaves the
// cache with room.
func TestCacheSetSucceedsWhileDeletesEmptyIt(t *testing.T) {
	t.Parallel()

	const sets = 100_000

	cache, err := New[int, int](1)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer cache.Reset()

	var (
		last  atomic.Int64
		stop  atomic.Bool
		group sync.WaitGroup
	)

	group.Go(func() {
		for !stop.Load() {
			cache.Delete(int(last.Load()))
		}
	})

	var (
		failed   int
		firstErr error
	)

	for key := 1; key <= sets; key++ {
		err = cache.Set(key, key)
		if err != nil {
			if failed == 0 {
				firstErr = err
			}

			failed++
		}

		last.Store(int64(key))
	}

	stop.Store(true)
	group.Wait()

	if failed > 0 {
		t.Fatalf("%d of %d Sets failed; first error: %v", failed, sets, firstErr)
	}
}

// TestGetDoesNotWaitForWriters checks that Get takes no lock that writers
// take. Taking one would make every Get write the lock's cache line, which
// serializes the Gets of one key across CPUs.
func TestGetDoesNotWaitForWriters(t *testing.T) {
	t.Parallel()

	c, err := New[string, int](16)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	if err := c.Set("k", 1); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	sh := &c.shards[c.shardIndexFromHash(c.hasher("k"))]

	// A writer that adds a key holds both locks.
	c.orderMu.Lock()
	sh.mu.Lock()

	var once sync.Once

	unlock := func() {
		once.Do(func() {
			sh.mu.Unlock()
			c.orderMu.Unlock()
		})
	}
	defer unlock()

	type found struct{ stored, absent bool }

	done := make(chan found, 1)

	go func() {
		_, stored := c.Get("k")
		_, absent := c.Get("absent")

		done <- found{stored, absent}
	}()

	select {
	case got := <-done:
		if !got.stored || got.absent {
			t.Fatalf("Get found a stored key: %v; found an absent key: %v", got.stored, got.absent)
		}
	case <-time.After(time.Second):
		t.Fatal("Get waited for a lock held by a writer")
	}

	unlock()

	var stats Stats

	c.UpdateStats(&stats)

	if stats.GetCalls != 2 || stats.Hits != 1 || stats.Misses != 1 {
		t.Fatalf("unexpected stats; got %+v; want 2 GetCalls, 1 Hit, 1 Miss", stats)
	}
}

// TestGetFindsKeysWhileDeletesMoveThem checks that Get never misses a stored
// key because a delete moved it. All keys share one hash, so they form one
// probe run in insertion order. The writer deletes the key at the front of the
// run and sets it again at the back, which moves every other key back one
// slot. Each reader gets the key half a run behind the front, which stays
// stored until keys/2 rotations later.
func TestGetFindsKeysWhileDeletesMoveThem(t *testing.T) {
	t.Parallel()

	const keys = 64

	c, err := New[int, int](16 * keys)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	c.hasher = func(int) uint64 { return 7 }

	for k := range keys {
		if err := c.Set(k, k); err != nil {
			t.Fatalf("Set(%d) error: %s", k, err)
		}
	}

	var (
		rotations atomic.Int64
		stop      atomic.Bool
		group     sync.WaitGroup
	)

	group.Go(func() {
		for n := 0; !stop.Load(); n++ {
			k := n % keys

			c.Delete(k)

			if err := c.Set(k, k); err != nil {
				t.Errorf("Set(%d) error: %s", k, err)

				return
			}

			rotations.Add(1)
		}
	})

	for range 4 {
		group.Go(func() {
			for !stop.Load() {
				before := rotations.Load()
				k := int((before + keys/2) % keys)

				if _, ok := c.Get(k); !ok && rotations.Load()-before < keys/2 {
					t.Errorf("Get(%d) missed a stored key while deletes moved it", k)

					return
				}
			}
		})
	}

	time.Sleep(200 * time.Millisecond)
	stop.Store(true)
	group.Wait()
}

// TestGetDoesNotAllocate is not parallel: AllocsPerRun counts the mallocs of
// the whole process.
func TestGetDoesNotAllocate(t *testing.T) {
	c, err := New[string, string](16)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	if err := c.Set("k", "v"); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	// A serial cache counts on one stripe; a contended one switches to the
	// pool of stripe indexes for good. Check both ways of counting.
	for _, striped := range []bool{false, true} {
		c.getsStriped.Store(striped)

		// Two collections empty the pool, so the run below would allocate
		// if Get had to box a stripe index.
		runtime.GC()
		runtime.GC()

		allocs := testing.AllocsPerRun(1000, func() {
			c.Get("k")
			c.Get("absent")
		})
		if allocs != 0 {
			t.Fatalf("Get allocated %v times per run with striped counters %v; want 0", allocs, striped)
		}
	}
}

func TestStatsCountConcurrentGetsExactly(t *testing.T) {
	t.Parallel()

	c, err := New[string, int](16)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	if err := c.Set("k", 1); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	const (
		goroutines = 8
		gets       = 10000
	)

	var group sync.WaitGroup
	for range goroutines {
		group.Go(func() { getOneInFour(c, gets) })
	}

	group.Wait()

	// A GetOrSet that loads counts as a hit.
	if _, loaded, err := c.GetOrSet("k", 2); err != nil || !loaded {
		t.Fatalf("GetOrSet returned loaded=%v, error=%v; want loaded", loaded, err)
	}

	var stats Stats

	c.UpdateStats(&stats)

	wantMisses := uint64(goroutines * gets / 4)
	wantCalls := uint64(goroutines*gets + 1)

	if stats.GetCalls != wantCalls || stats.Misses != wantMisses || stats.Hits != wantCalls-wantMisses {
		t.Fatalf("unexpected stats; got %+v; want %d GetCalls, %d Misses", stats, wantCalls, wantMisses)
	}
}

// TestCacheResetClearsGetStats checks that Reset zeroes every stripe of the Get
// counts, not only the one that serial Gets count on.
func TestCacheResetClearsGetStats(t *testing.T) {
	t.Parallel()

	c, err := New[string, int](16)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	if err := c.Set("k", 1); err != nil {
		t.Fatalf("Set error: %s", err)
	}

	c.Get("k")
	c.Get("absent")
	c.gets[len(c.gets)-1].misses.Add(1) // as a contended Get may count
	c.Reset()

	var stats Stats

	c.UpdateStats(&stats)

	if stats.GetCalls != 0 || stats.Misses != 0 {
		t.Fatalf("unexpected stats after Reset; got %+v; want no GetCalls or Misses", stats)
	}
}

// getOneInFour gets "absent" for every fourth of n calls, and "k" for the rest.
func getOneInFour(c *Cache[string, int], n int) {
	for i := range n {
		if i%4 == 0 {
			c.Get("absent")
		} else {
			c.Get("k")
		}
	}
}

// TestGetConcurrentWithWritersIsConsistent runs Get against updates, deletes,
// evictions, table growth, and Reset. A Get must return a value stored for
// its key, and never an older version than the same goroutine saw before.
func TestGetConcurrentWithWritersIsConsistent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		hasher func(string) uint64
	}{
		{"rapidhash", nil},
		{"one hash", func(string) uint64 { return 7 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testGetConcurrentWithWriters(t, tc.hasher)
		})
	}
}

func testGetConcurrentWithWriters(t *testing.T, hasher func(string) uint64) {
	t.Helper()

	const (
		keys       = 64
		maxEntries = 48
		writers    = 4
		readers    = 8
	)

	c, err := New[string, [2]int](maxEntries)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	if hasher != nil {
		c.hasher = hasher
	}

	names := make([]string, keys)
	for i := range names {
		names[i] = fmt.Sprintf("key %d", i)
	}

	var (
		stop  atomic.Bool
		group sync.WaitGroup
	)

	// Each key has one writer, so its versions only increase.
	for first := range writers {
		group.Go(func() { writeVersions(t, c, names, first, writers, &stop) })
	}

	group.Go(func() {
		for !stop.Load() {
			time.Sleep(time.Millisecond)
			c.Reset()
		}
	})

	for range readers {
		group.Go(func() {
			seen := make([]int, keys)

			for !stop.Load() {
				if !readVersions(t, c, names, seen) {
					return
				}
			}
		})
	}

	time.Sleep(300 * time.Millisecond)
	stop.Store(true)
	group.Wait()
}

// writeVersions stores a new version of every step-th name from names[first],
// until stop. It deletes the name instead on every seventh version, so that
// deletes move other entries.
func writeVersions(
	t *testing.T, c *Cache[string, [2]int], names []string, first, step int, stop *atomic.Bool,
) {
	t.Helper()

	for ver := 1; !stop.Load(); ver++ {
		for i := first; i < len(names); i += step {
			if ver%7 == 3 {
				c.Delete(names[i])

				continue
			}

			err := c.Set(names[i], [2]int{i, ver})
			if err != nil {
				t.Errorf("Set(%q) error: %s", names[i], err)

				return
			}
		}
	}
}

// readVersions gets every name and records in seen the version it found. It
// reports an error, and returns false, for a value stored for another key or
// older than the one in seen.
func readVersions(t *testing.T, c *Cache[string, [2]int], names []string, seen []int) bool {
	t.Helper()

	for i, name := range names {
		got, ok := c.Get(name)
		if !ok {
			continue
		}

		if got[0] != i || got[1] < seen[i] {
			t.Errorf("Get(%q) = %v after seeing version %d", name, got, seen[i])

			return false
		}

		seen[i] = got[1]
	}

	return true
}
