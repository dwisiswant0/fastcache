package fastcache

import (
	"fmt"
	"iter"
	"slices"
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
	order      slotRing
	seq        uint64       // last insertion number, guarded by orderMu
	entryCount atomic.Int64 // global entry count for accurate capacity enforcement

	// Every Get reads gets and getsStriped. The padding keeps them off the
	// cache lines that writers dirty: the fields above, and whatever the
	// allocator places after a Cache.
	_           [128]byte
	gets        []getCounter // Get hits and misses, striped across CPUs
	getsStriped atomic.Bool  // set by the first Gets to contend on gets[0]
	_           [128]byte
}

// slot records one insertion in FIFO order. Delete leaves slots in place, so
// a slot matches its entry by seq, not by key: a key that is deleted and set
// again must not be evicted through the slot of its first insertion.
type slot struct {
	shard int
	hash  uint64
	seq   uint64
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

// minCompactSlots is the fewest reclaimable slots worth a compaction pass
// over the order log.
const minCompactSlots = 1024

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
		gets:       newGetCounters(),
	}

	// Build the empty state through Reset, so New and Reset share one
	// definition of it. maxEntries is only a cap, so storage grows with
	// the stored entries.
	c.Reset()

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

	if e := c.shards[idx].lookup(h, k); e != nil {
		c.countGet(true)

		return e.Value, true
	}

	c.countGet(false)

	var zero V

	return zero, false
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

// Reset removes all the items from the cache and releases the memory they
// used.
func (c *Cache[K, V]) Reset() {
	c.orderMu.Lock()
	for i := range c.shards {
		c.shards[i].reset()
	}

	c.order = slotRing{}
	c.entryCount.Store(0)

	// Get adds to gets without a lock, so Reset zeroes the cells in place
	// instead of replacing the slice.
	for i := range c.gets {
		c.gets[i].hits.Store(0)
		c.gets[i].misses.Store(0)
	}
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

	c.dropDeletedLocked()

	for {
		shard := &c.shards[idx]
		shard.mu.Lock()

		if t, pos, e := shard.findLocked(hash, k); pos >= 0 {
			result, err := c.handleExisting(op, shard, t, pos, e, v)
			shard.mu.Unlock()

			return result, err
		}

		if c.entryCount.Load() < int64(c.maxEntries) {
			result, err := c.handleInsert(op, idx, hash, k, v, shard)
			shard.mu.Unlock()

			return result, err
		}
		shard.mu.Unlock()

		if !c.evictOldestLocked() {
			return result[V]{}, fmt.Errorf("%w: entry count=%d, max entries=%d", ErrEvictionFailed, c.entryCount.Load(), c.maxEntries)
		}
	}
}

func (c *Cache[K, V]) handleExisting(
	op op, shard *shard[K, V], t *table[K, V], pos int, e *entry[K, V], v V,
) (result[V], error) {
	switch op {
	case opSet:
		shard.replaceLocked(t, pos, e, v)

		return result[V]{}, nil
	case opGetOrSet:
		c.countGet(true)

		return result[V]{value: e.Value, loaded: true}, nil
	case opSetIfAbsent:
		return result[V]{}, nil
	default:
		return result[V]{}, fmt.Errorf("%w: %d", errUnknownOp, op)
	}
}

func (c *Cache[K, V]) handleInsert(op op, idx int, hash uint64, k K, v V, shard *shard[K, V]) (result[V], error) {
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

	c.seq++
	shard.insertLocked(&entry[K, V]{Key: k, Value: v, seq: c.seq, hash: hash})
	c.order.push(slot{shard: idx, hash: hash, seq: c.seq})
	c.entryCount.Add(1)

	return res, nil
}

func (c *Cache[K, V]) evictOldestLocked() bool {
	for c.order.len > 0 {
		slot := c.order.pop()

		shard := &c.shards[slot.shard]
		shard.mu.Lock()
		if t, pos := shard.findSeqLocked(slot.hash, slot.seq); pos >= 0 {
			shard.removeLocked(t, pos)
			shard.evictions++
			shard.mu.Unlock()
			c.entryCount.Add(-1)

			return true
		}
		shard.mu.Unlock()
	}

	return false
}

// dropDeletedLocked removes the slots of deleted entries from the order log
// once they outnumber live entries by minCompactSlots. Delete leaves its slot
// behind, so without this pass Set and Delete churn grows the log without
// bound, and the next eviction has to skip every dead slot. Each pass removes
// at least as many slots as it keeps, so its cost stays O(1) per Delete.
//
// Callers must not hold any shard lock.
func (c *Cache[K, V]) dropDeletedLocked() {
	live := int(c.entryCount.Load())
	if c.order.len-live <= live+minCompactSlots {
		return
	}

	c.order.retain(c.slotLive)
}

func (c *Cache[K, V]) slotLive(s slot) bool {
	shard := &c.shards[s.shard]
	shard.mu.Lock()
	_, pos := shard.findSeqLocked(s.hash, s.seq)
	live := pos >= 0
	shard.mu.Unlock()

	return live
}

// slotRing is the order log: a FIFO queue of slots in a ring buffer. Popping
// the oldest slot frees its cell in place. A plain slice would have to move
// every remaining slot to reuse its front, and an eviction would then cost
// time in proportion to the cache size.
type slotRing struct {
	buf  []slot
	head int // index in buf of the oldest slot
	len  int // number of slots in the queue
}

// push adds s as the newest slot.
func (r *slotRing) push(s slot) {
	if r.len == len(r.buf) {
		r.grow()
	}

	r.buf[r.index(r.len)] = s
	r.len++
}

// pop removes and returns the oldest slot. The ring must not be empty.
func (r *slotRing) pop() slot {
	s := r.buf[r.head]
	r.head = r.index(1)
	r.len--

	return s
}

// retain removes the slots for which keep returns false and keeps the others
// in order.
func (r *slotRing) retain(keep func(slot) bool) {
	kept := 0

	for i := range r.len {
		if s := r.buf[r.index(i)]; keep(s) {
			r.buf[r.index(kept)] = s
			kept++
		}
	}

	r.len = kept
}

// index returns the position in buf of the i-th oldest slot.
func (r *slotRing) index(i int) int {
	i += r.head
	if i >= len(r.buf) {
		i -= len(r.buf)
	}

	return i
}

// grow enlarges buf as append would grow a slice, so the ring takes as much
// memory as a slice of the same slots did. When the queue wraps around, the
// oldest slots run from head to the end of buf; they move to the end of the
// new buffer, which leaves the free cells between the newest and the oldest
// slot.
func (r *slotRing) grow() {
	buf := slices.Grow(r.buf, 1)
	buf = buf[:cap(buf)]

	if r.head > 0 {
		head := len(buf) - (len(r.buf) - r.head)
		copy(buf[head:], r.buf[r.head:])
		r.head = head
	}

	r.buf = buf
}
