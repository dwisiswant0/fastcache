package fastcache

import (
	"fmt"
	"iter"
	"sync"
	"sync/atomic"

	"go.dw1.io/rapidhash"
)

// Cache is a fast thread-safe in-memory cache with FIFO eviction.
//
// Call [Cache.Reset] when the cache is no longer needed. This reclaims the allocated
// memory.
type Cache[K comparable, V any] struct {
	shards     [shardsCount]shard[K, V]
	hasher     func(K) uint64
	maxEntries int
	orderMu    sync.Mutex
	order      []slot[K]
	head       int
	entryCount atomic.Int64 // global entry count for accurate capacity enforcement
}

type slot[K comparable] struct {
	shard int
	hash  uint64
	key   K
}

type op uint8

const (
	opSet op = iota
	opGetOrSet
	opSetIfAbsent
)

type result[V any] struct {
	value  V
	loaded bool
	stored bool
}

const shardMask = uint64(shardsCount - 1)

// New returns a new cache with the given maxEntries capacity.
//
// maxEntries is the maximum number of entries the cache can hold.
// When the cache is full, the oldest entries are evicted (FIFO).
//
// New returns an error if maxEntries is not positive.
func New[K comparable, V any](maxEntries int) (*Cache[K, V], error) {
	if maxEntries <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidMaxEntries, maxEntries)
	}

	c := &Cache[K, V]{
		maxEntries: maxEntries,
		hasher:     newHasher[K](),
		order:      make([]slot[K], 0, maxEntries),
	}

	entriesPerShard := (maxEntries + shardsCount - 1) / shardsCount
	for i := range c.shards {
		c.shards[i].entries = make(map[uint64][]entry[K, V], entriesPerShard)
	}

	return c, nil
}

// Set stores (k, v) in the cache.
//
// The stored entry may be evicted at any time due to cache overflow.
//
// Set returns an error if the cache cannot evict an existing entry while full.
func (c *Cache[K, V]) Set(k K, v V) error {
	h := c.hasher(k)
	idx := c.shardIndexFromHash(h)

	return c.shards[idx].set(c, idx, h, k, v)
}

// Get returns the value for the given key.
//
// Returns the zero value and false if the key is not found.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	h := c.hasher(k)
	idx := c.shardIndexFromHash(h)

	return c.shards[idx].get(h, k)
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
//
// GetOrSet returns an error if the cache cannot evict an existing entry while full.
func (c *Cache[K, V]) GetOrSet(k K, v V) (actual V, loaded bool, err error) {
	h := c.hasher(k)
	idx := c.shardIndexFromHash(h)

	return c.shards[idx].getOrSet(c, idx, h, k, v)
}

// SetIfAbsent stores the value for a key only if the key is not already present.
//
// Returns true if the value was stored, false if the key already existed.
//
// SetIfAbsent returns an error if the cache cannot evict an existing entry while full.
func (c *Cache[K, V]) SetIfAbsent(k K, v V) (stored bool, err error) {
	h := c.hasher(k)
	idx := c.shardIndexFromHash(h)

	return c.shards[idx].setIfAbsent(c, idx, h, k, v)
}

// Delete removes the value for the given key.
func (c *Cache[K, V]) Delete(k K) {
	h := c.hasher(k)
	idx := c.shardIndexFromHash(h)
	c.shards[idx].delete(c, h, k)
}

// GetAndDelete deletes the value for a key, returning the previous value if any.
//
// The loaded result reports whether the key was present.
func (c *Cache[K, V]) GetAndDelete(k K) (v V, loaded bool) {
	h := c.hasher(k)
	idx := c.shardIndexFromHash(h)

	return c.shards[idx].getAndDelete(c, h, k)
}

// Reset removes all the items from the cache.
func (c *Cache[K, V]) Reset() {
	c.orderMu.Lock()
	for i := range c.shards {
		c.shards[i].reset()
	}
	clear(c.order)
	c.order = c.order[:0]
	c.head = 0
	c.entryCount.Store(0)
	c.orderMu.Unlock()
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

func (c *Cache[K, V]) shardIndexFromHash(h uint64) int {
	return int(h & shardMask)
}

func newHasher[K comparable]() func(K) uint64 {
	var zero K
	if _, ok := any(zero).(string); ok {
		return hashStringKey[K]
	}

	return rapidhash.HashComparable[K]
}

func hashStringKey[K comparable](k K) uint64 {
	return rapidhash.HashString(any(k).(string))
}

func (c *Cache[K, V]) runInsert(op op, idx int, hash uint64, k K, v V) (result[V], error) {
	c.orderMu.Lock()
	defer c.orderMu.Unlock()

	for {
		shard := &c.shards[idx]
		shard.mu.Lock()

		bucket := shard.entries[hash]
		if pos := findEntry(bucket, k); pos >= 0 {
			result, err := c.handleExisting(op, shard, bucket, pos, v)
			shard.mu.Unlock()

			return result, err
		}

		if c.entryCount.Load() < int64(c.maxEntries) {
			result, err := c.handleInsert(op, idx, hash, k, v, shard, bucket)
			shard.mu.Unlock()

			return result, err
		}
		shard.mu.Unlock()

		if !c.evictOldestLocked() {
			return result[V]{}, fmt.Errorf("%w: entry count=%d, max entries=%d", ErrEvictionFailed, c.entryCount.Load(), c.maxEntries)
		}
	}
}

func (c *Cache[K, V]) handleExisting(op op, shard *shard[K, V], bucket []entry[K, V], pos int, v V) (result[V], error) {
	switch op {
	case opSet:
		bucket[pos].Value = v

		return result[V]{}, nil
	case opGetOrSet:
		shard.getCalls++

		return result[V]{value: bucket[pos].Value, loaded: true}, nil
	case opSetIfAbsent:
		return result[V]{}, nil
	default:
		return result[V]{}, fmt.Errorf("%w: %d", errUnknownOp, op)
	}
}

func (c *Cache[K, V]) handleInsert(op op, idx int, hash uint64, k K, v V, shard *shard[K, V], bucket []entry[K, V]) (result[V], error) {
	var res result[V]

	switch op {
	case opSet:
		res = result[V]{}
	case opGetOrSet:
		shard.setCalls++
		res = result[V]{value: v}
	case opSetIfAbsent:
		shard.setCalls++
		res = result[V]{stored: true}
	default:
		return result[V]{}, fmt.Errorf("%w: %d", errUnknownOp, op)
	}

	shard.entries[hash] = append(bucket, entry[K, V]{Key: k, Value: v})
	shard.entryCount++
	c.order = append(c.order, slot[K]{shard: idx, hash: hash, key: k})
	c.entryCount.Add(1)

	return res, nil
}

func (c *Cache[K, V]) evictOldestLocked() bool {
	for c.head < len(c.order) {
		slot := c.order[c.head]
		c.head++

		shard := &c.shards[slot.shard]
		shard.mu.Lock()
		bucket := shard.entries[slot.hash]
		if pos := findEntry(bucket, slot.key); pos >= 0 {
			bucket = deleteEntry(bucket, pos)
			if len(bucket) == 0 {
				delete(shard.entries, slot.hash)
			} else {
				shard.entries[slot.hash] = bucket
			}
			shard.entryCount--
			shard.evictions++
			shard.mu.Unlock()
			c.entryCount.Add(-1)
			c.compactOrderLocked()

			return true
		}
		shard.mu.Unlock()
	}

	return false
}

func (c *Cache[K, V]) compactOrderLocked() {
	if c.head < 1024 && c.head*2 < len(c.order) {
		return
	}

	remaining := len(c.order) - c.head
	copy(c.order, c.order[c.head:])
	clear(c.order[remaining:])
	c.order = c.order[:remaining]
	c.head = 0
}
