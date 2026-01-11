package fastcache

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCacheSmall(t *testing.T) {
	c := New[string, string](100)
	defer c.Reset()

	if _, ok := c.Get("aaa"); ok {
		t.Fatalf("unexpected value found for non-existent key")
	}

	c.Set("key", "value")
	if v, ok := c.Get("key"); !ok || v != "value" {
		t.Fatalf("unexpected value obtained; got %q; want %q", v, "value")
	}
	if _, ok := c.Get(""); ok {
		t.Fatalf("unexpected value found for empty key")
	}
	if _, ok := c.Get("aaa"); ok {
		t.Fatalf("unexpected value found for non-existent key")
	}

	c.Set("aaa", "bbb")
	if v, ok := c.Get("aaa"); !ok || v != "bbb" {
		t.Fatalf("unexpected value obtained; got %q; want %q", v, "bbb")
	}

	c.Reset()
	if _, ok := c.Get("aaa"); ok {
		t.Fatalf("unexpected value found after reset")
	}

	// Test empty value
	k := "empty"
	c.Set(k, "")
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
	c := New[string, []byte](100)
	defer c.Reset()

	key := "key"
	value := []byte("value")

	c.Set(key, value)
	if v, ok := c.Get(key); !ok || string(v) != string(value) {
		t.Fatalf("unexpected value obtained; got %q; want %q", v, value)
	}
}

func TestCacheWrap(t *testing.T) {
	c := New[string, string](1000)
	defer c.Reset()

	calls := 5000

	for i := 0; i < calls; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		c.Set(k, v)
		vv, ok := c.Get(k)
		if !ok || vv != v {
			t.Fatalf("unexpected value for key %q; got %q; want %q", k, vv, v)
		}
	}

	// Some entries may have been evicted
	hits := 0
	for i := 0; i < calls; i++ {
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
	c := New[string, string](1024)
	defer c.Reset()

	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		c.Set(k, v)
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

func TestCacheSetGetSerial(t *testing.T) {
	itemsCount := 10000
	c := New[string, string](itemsCount * 2)
	defer c.Reset()
	if err := testCacheGetSet(c, itemsCount); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestCacheGetSetConcurrent(t *testing.T) {
	itemsCount := 10000
	const goroutines = 10
	c := New[string, string](itemsCount * goroutines * 2)
	defer c.Reset()

	ch := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			ch <- testCacheGetSet(c, itemsCount)
		}()
	}
	for i := 0; i < goroutines; i++ {
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
	for i := 0; i < itemsCount; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		c.Set(k, v)
		vv, ok := c.Get(k)
		if !ok || vv != v {
			return fmt.Errorf("unexpected value for key %q after insertion; got %q; want %q", k, vv, v)
		}
	}
	misses := 0
	for i := 0; i < itemsCount; i++ {
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
	c := New[string, string](12334)

	stopCh := make(chan struct{})

	// run workers for cache reset
	var resettersWG sync.WaitGroup
	for i := 0; i < 10; i++ {
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
	for i := 0; i < 10; i++ {
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
	for i := 0; i < 10; i++ {
		settersWG.Add(1)
		go func() {
			defer settersWG.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key_%d", j)
				value := fmt.Sprintf("value_%d", j)
				c.Set(key, value)
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
	c := New[string, string](100)
	defer c.Reset()

	// Add entries
	for i := 0; i < 50; i++ {
		c.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i))
	}

	// Count entries via Range
	count := 0
	c.Range(func(k string, v string) bool {
		count++
		return true
	})

	if count != 50 {
		t.Fatalf("unexpected count from Range; got %d; want 50", count)
	}

	// Test early exit
	count = 0
	c.Range(func(k string, v string) bool {
		count++
		return count < 10
	})

	if count != 10 {
		t.Fatalf("unexpected count with early exit; got %d; want 10", count)
	}
}

func TestCacheLen(t *testing.T) {
	c := New[string, string](100)
	defer c.Reset()

	if c.Len() != 0 {
		t.Fatalf("unexpected len for empty cache; got %d; want 0", c.Len())
	}

	for i := 0; i < 50; i++ {
		c.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i))
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

	c := New[int, User](100)
	defer c.Reset()

	u := User{ID: 1, Name: "Alice"}
	c.Set(1, u)

	got, ok := c.Get(1)
	if !ok {
		t.Fatal("user not found")
	}
	if got.ID != u.ID || got.Name != u.Name {
		t.Fatalf("unexpected user; got %+v; want %+v", got, u)
	}
}
