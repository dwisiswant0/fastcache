package fastcache

import (
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"iter"
	"sync/atomic"
)

// Cache is a fast thread-safe in-memory cache.
//
// Call [Cache.Reset] when the cache is no longer needed. This reclaims the allocated
// memory.
type Cache[K comparable, V any] struct {
	shards     [shardsCount]shard[K, V]
	maxEntries int
	entryCount atomic.Int64 // global entry count for accurate capacity enforcement
}

// keySlot holds a key and whether it's valid (not deleted)
type keySlot[K comparable] struct {
	key   K
	valid bool
}

const (
	shardMask             = shardsCount - 1
	stringShardSampleSize = 16
	stringShardSamples    = 4
)

// New returns a new cache with the given maxEntries capacity.
//
// maxEntries is the maximum number of entries the cache can hold.
// When the cache is full, the oldest entries are evicted (FIFO).
func New[K comparable, V any](maxEntries int) *Cache[K, V] {
	if maxEntries <= 0 {
		panic(fmt.Errorf("maxEntries must be greater than 0; got %d", maxEntries))
	}

	c := &Cache[K, V]{
		maxEntries: maxEntries,
	}

	entriesPerShard := (maxEntries + shardsCount - 1) / shardsCount
	inlineCap := min(entriesPerShard, inlineShardEntries)

	for i := range c.shards {
		if inlineCap > 0 {
			c.shards[i].smallEntries = make([]inlineEntry[K, V], 0, inlineCap)
		}
		c.shards[i].order = make([]keySlot[K], entriesPerShard)
	}

	return c
}

// Set stores (k, v) in the cache.
//
// The stored entry may be evicted at any time due to cache overflow.
func (c *Cache[K, V]) Set(k K, v V) {
	idx := shardIndex(k)
	c.shards[idx].set(c, k, v)
}

// Get returns the value for the given key.
//
// Returns the zero value and false if the key is not found.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	idx := shardIndex(k)

	return c.shards[idx].get(k)
}

// Has returns true if entry for the given key exists in the cache.
func (c *Cache[K, V]) Has(k K) bool {
	_, ok := c.Get(k)

	return ok
}

// GetOrSet returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
//
// The loaded result is true if the value was loaded, false if stored.
func (c *Cache[K, V]) GetOrSet(k K, v V) (actual V, loaded bool) {
	idx := shardIndex(k)

	return c.shards[idx].getOrSet(c, k, v)
}

// SetIfAbsent stores the value for a key only if the key is not already present.
//
// Returns true if the value was stored, false if the key already existed.
func (c *Cache[K, V]) SetIfAbsent(k K, v V) (stored bool) {
	idx := shardIndex(k)

	return c.shards[idx].setIfAbsent(c, k, v)
}

// Delete removes the value for the given key.
func (c *Cache[K, V]) Delete(k K) {
	idx := shardIndex(k)
	c.shards[idx].delete(c, k)
}

// GetAndDelete deletes the value for a key, returning the previous value if any.
//
// The loaded result reports whether the key was present.
func (c *Cache[K, V]) GetAndDelete(k K) (v V, loaded bool) {
	idx := shardIndex(k)

	return c.shards[idx].getAndDelete(c, k)
}

// Reset removes all the items from the cache.
func (c *Cache[K, V]) Reset() {
	for i := range c.shards {
		c.shards[i].reset()
	}
	c.entryCount.Store(0)
}

// Len returns the number of entries in the cache.
func (c *Cache[K, V]) Len() int {
	return int(c.entryCount.Load())
}

// All returns an iterator over all key-value pairs in the cache.
//
// Note: It's safe to call other cache methods during iteration,
// but the iteration may not reflect concurrent modifications.
func (c *Cache[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for i := range c.shards {
			if !c.shards[i].rangeEntries(yield) {
				return
			}
		}
	}
}

// Keys returns an iterator over all keys in the cache.
//
// Note: It's safe to call other cache methods during iteration,
// but the iteration may not reflect concurrent modifications.
func (c *Cache[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for i := range c.shards {
			if !c.shards[i].rangeKeys(yield) {
				return
			}
		}
	}
}

// Values returns an iterator over all values in the cache.
//
// Note: It's safe to call other cache methods during iteration,
// but the iteration may not reflect concurrent modifications.
func (c *Cache[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		for i := range c.shards {
			if !c.shards[i].rangeValues(yield) {
				return
			}
		}
	}
}

// hashSeed is the seed used for hashing keys.
var hashSeed = maphash.MakeSeed()

func shardIndex[K comparable](k K) int {
	return int(hashKey(k) & shardMask)
}

// hashKey returns a hash for the given key.
func hashKey[K comparable](k K) uint64 {
	if s, ok := any(k).(string); ok {
		return hashStringKey(s)
	}

	return maphash.Comparable(hashSeed, k)
}

func hashStringKey(s string) uint64 {
	if len(s) <= stringShardSamples*stringShardSampleSize {
		return maphash.String(hashSeed, s)
	}

	var h maphash.Hash
	h.SetSeed(hashSeed)

	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(s)))
	_, _ = h.Write(lenBuf[:])

	starts := [stringShardSamples]int{
		0,
		len(s)/3 - stringShardSampleSize/2,
		(2*len(s))/3 - stringShardSampleSize/2,
		len(s) - stringShardSampleSize,
	}
	for _, start := range starts {
		if start < 0 {
			start = 0
		}

		end := start + stringShardSampleSize
		if end > len(s) {
			end = len(s)
			start = end - stringShardSampleSize
		}

		_, _ = h.WriteString(s[start:end])
	}

	return h.Sum64()
}
