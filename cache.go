package fastcache

import (
	"fmt"
	"hash/maphash"
	"iter"
	"sync"
	"sync/atomic"
)

const shardsCount = 512

// Stats represents cache stats.
//
// Use [Cache.UpdateStats] for obtaining fresh stats from the cache.
type Stats struct {
	// GetCalls is the number of Get calls.
	GetCalls uint64

	// SetCalls is the number of Set calls.
	SetCalls uint64

	// Misses is the number of cache misses.
	Misses uint64

	// Hits is the number of cache hits.
	Hits uint64

	// Deletes is the number of Delete calls.
	Deletes uint64

	// Evictions is the number of entries evicted due to capacity limits.
	Evictions uint64

	// EntriesCount is the current number of entries in the cache.
	EntriesCount uint64

	// MaxEntries is the maximum number of entries allowed in the cache.
	MaxEntries uint64
}

// Reset resets s, so it may be re-used again in [Cache.UpdateStats].
func (s *Stats) Reset() {
	*s = Stats{}
}

// Cache is a fast thread-safe in-memory cache with FIFO eviction.
//
// Call [Reset] when the cache is no longer needed. This reclaims the allocated
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

type shard[K comparable, V any] struct {
	mu sync.RWMutex

	// stats (hits computed as getCalls - misses)
	getCalls  uint64
	setCalls  uint64
	misses    uint64
	deletes   uint64
	evictions uint64

	// entries maps keys to values
	entries map[K]V

	// ring buffer for FIFO eviction order
	order    []keySlot[K] // circular buffer of keys
	writeIdx int          // next write pos
}

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
	for i := range c.shards {
		c.shards[i].entries = make(map[K]V, entriesPerShard)
		c.shards[i].order = make([]keySlot[K], entriesPerShard)
	}

	return c
}

// Set stores (k, v) in the cache.
//
// The stored entry may be evicted at any time due to cache overflow.
func (c *Cache[K, V]) Set(k K, v V) {
	h := hashKey(k)
	idx := h % shardsCount
	c.shards[idx].set(c, k, v)
}

func (s *shard[K, V]) set(c *Cache[K, V], k K, v V) {
	atomic.AddUint64(&s.setCalls, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update existing key - no count change
	if _, exists := s.entries[k]; exists {
		s.entries[k] = v

		return
	}

	maxIter := len(s.order)
	for i := 0; i < maxIter && c.entryCount.Load() >= int64(c.maxEntries) && len(s.entries) > 0; i++ {
		slot := &s.order[s.writeIdx]
		if slot.valid {
			if _, exists := s.entries[slot.key]; exists {
				delete(s.entries, slot.key)
				slot.valid = false
				c.entryCount.Add(-1)
				atomic.AddUint64(&s.evictions, 1)
			} else {
				slot.valid = false
			}
		}
		s.writeIdx = (s.writeIdx + 1) % len(s.order)
	}

	s.order[s.writeIdx] = keySlot[K]{key: k, valid: true}
	s.writeIdx = (s.writeIdx + 1) % len(s.order)
	s.entries[k] = v
	c.entryCount.Add(1)
}

// Get returns the value for the given key.
//
// Returns the zero value and false if the key is not found.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	h := hashKey(k)
	idx := h % shardsCount

	return c.shards[idx].get(k)
}

func (s *shard[K, V]) get(k K) (V, bool) {
	s.mu.RLock()
	atomic.AddUint64(&s.getCalls, 1)
	v, ok := s.entries[k]
	s.mu.RUnlock()

	if !ok {
		atomic.AddUint64(&s.misses, 1)
	}
	// NOTE(dwisiswant0): hits = getCalls - misses (computed in [UpdateStats]).

	return v, ok
}

// Has returns true if entry for the given key exists in the cache.
func (c *Cache[K, V]) Has(k K) bool {
	_, ok := c.Get(k)
	return ok
}

// Delete removes the value for the given key.
func (c *Cache[K, V]) Delete(k K) {
	h := hashKey(k)
	idx := h % shardsCount
	c.shards[idx].delete(c, k)
}

func (s *shard[K, V]) delete(c *Cache[K, V], k K) {
	atomic.AddUint64(&s.deletes, 1)

	s.mu.Lock()
	// NOTE(dwisiswant0): we don't remove from the ring buffer here.
	// The slot will be marked invalid when we encounter it during eviction.
	// This is O(1) delete vs O(n) if we searched the ring buffer btw.
	if _, exists := s.entries[k]; exists {
		delete(s.entries, k)
		c.entryCount.Add(-1)
	}
	s.mu.Unlock()
}

// Reset removes all the items from the cache.
func (c *Cache[K, V]) Reset() {
	for i := range c.shards {
		c.shards[i].reset()
	}
	c.entryCount.Store(0)
}

func (s *shard[K, V]) reset() {
	s.mu.Lock()
	s.entries = make(map[K]V)
	for i := range s.order {
		s.order[i] = keySlot[K]{}
	}
	s.writeIdx = 0
	atomic.StoreUint64(&s.getCalls, 0)
	atomic.StoreUint64(&s.setCalls, 0)
	atomic.StoreUint64(&s.misses, 0)
	atomic.StoreUint64(&s.deletes, 0)
	atomic.StoreUint64(&s.evictions, 0)
	s.mu.Unlock()
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

func (s *shard[K, V]) rangeEntries(f func(k K, v V) bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for k, v := range s.entries {
		if !f(k, v) {
			return false
		}
	}

	return true
}

// UpdateStats adds cache stats to s.
//
// Call [Stats.Reset] before calling UpdateStats if s is re-used.
func (c *Cache[K, V]) UpdateStats(s *Stats) {
	for i := range c.shards {
		shard := &c.shards[i]
		s.GetCalls += atomic.LoadUint64(&shard.getCalls)
		s.SetCalls += atomic.LoadUint64(&shard.setCalls)
		s.Misses += atomic.LoadUint64(&shard.misses)
		s.Deletes += atomic.LoadUint64(&shard.deletes)
		s.Evictions += atomic.LoadUint64(&shard.evictions)
	}

	s.EntriesCount = uint64(c.entryCount.Load())
	s.Hits = s.GetCalls - s.Misses
	s.MaxEntries = uint64(c.maxEntries)
}

// hashSeed is the seed used for hashing keys.
var hashSeed = maphash.MakeSeed()

// hashKey returns a hash for the given key.
func hashKey[K comparable](k K) uint64 {
	return maphash.Comparable(hashSeed, k)
}
