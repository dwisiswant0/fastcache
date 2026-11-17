package fastcache

import (
	"cmp"
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

	// NewWeighted sets weigh and maxWeight. In a cache made by New, weigh is
	// nil and every entry weighs 0, so a maxWeight of 0 still fits them all.
	weigh     func(K, V) int
	maxWeight int
	weight    atomic.Int64 // total weight of the entries

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

// compactStep is how many slots of a compaction pass each insert scans. It
// exceeds the one slot that an insert adds, so the pass catches up with the
// log, and it stays small so that no insert holds orderMu for long.
const compactStep = 8

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

// NewWeighted returns a new cache that holds at most maxEntries entries whose
// weights add up to at most maxWeight.
//
// weigh returns the weight of an entry, such as the length of its value. The
// cache calls it before storing a value and keeps the weight until the entry
// leaves the cache, so later changes to data that the value points to do not
// change its weight.
//
// When a new or heavier value does not fit, the oldest entries are evicted
// (FIFO) until it does. A Set that makes a value heavier evicts only other
// entries, so its key stays in the cache and keeps its place. Storing a value
// whose weight is negative or greater than maxWeight returns
// [ErrInvalidWeight] and leaves the cache unchanged.
//
// To bound the cache by weight alone, pass [math.MaxInt] as maxEntries. The
// cache allocates storage as entries are stored, not up front.
//
// To restore a saved weighted cache, make a new one with NewWeighted and the
// same arguments, then pass its [Cache.Set] to [LoadFromFunc], which stops with
// [ErrInvalidWeight] at the first saved value heavier than maxWeight.
// [LoadFrom] and [LoadFromFile] drop the weight bound, so a cache saved with
// [math.MaxInt] as maxEntries loads with no bound at all.
//
// NewWeighted returns an error if maxEntries or maxWeight is not positive, or
// if weigh is nil.
func NewWeighted[K comparable, V any](maxEntries, maxWeight int, weigh func(K, V) int) (*Cache[K, V], error) {
	if maxWeight <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidMaxWeight, maxWeight)
	}

	if weigh == nil {
		return nil, errNilWeigh
	}

	c, err := New[K, V](maxEntries)
	if err != nil {
		return nil, err
	}

	c.weigh = weigh
	c.maxWeight = maxWeight

	return c, nil
}

// Set stores (k, v) in the cache.
//
// The stored entry may be evicted at any time due to cache overflow.
//
// Set returns an error if the cache cannot evict an existing entry while full.
// In a cache made by [NewWeighted], it returns [ErrInvalidWeight] if the
// weight of v is negative or greater than maxWeight.
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
// In a cache made by [NewWeighted], it returns [ErrInvalidWeight] if k is
// absent and the weight of v is negative or greater than maxWeight.
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
// In a cache made by [NewWeighted], it returns [ErrInvalidWeight] if k is
// absent and the weight of v is negative or greater than maxWeight.
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
	c.weight.Store(0)

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

// AllFIFO returns an iterator over all key-value pairs in the cache in FIFO
// order: oldest first, which is the order in which the cache evicts them.
// Updating a key keeps its place, while deleting a key and setting it again
// makes it the newest.
//
// Unlike [Cache.All], AllFIFO copies and sorts every entry before it yields
// the first pair, even when the loop stops early, and holds memory in
// proportion to [Cache.Len] until the iteration ends.
//
// Note: It's safe to call other cache methods during iteration,
// but the iteration may not reflect concurrent modifications.
func (c *Cache[K, V]) AllFIFO() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, e := range c.entriesOldestFirst(1) {
			if !yield(e.Key, e.Value) {
				return
			}
		}
	}
}

// KeysFIFO returns an iterator over all keys in the cache in FIFO order,
// oldest first. It costs as much as [Cache.AllFIFO].
//
// Note: It's safe to call other cache methods during iteration,
// but the iteration may not reflect concurrent modifications.
func (c *Cache[K, V]) KeysFIFO() iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range c.AllFIFO() {
			if !yield(k) {
				return
			}
		}
	}
}

// ValuesFIFO returns an iterator over all values in the cache in FIFO order,
// oldest first. It costs as much as [Cache.AllFIFO].
//
// Note: It's safe to call other cache methods during iteration,
// but the iteration may not reflect concurrent modifications.
func (c *Cache[K, V]) ValuesFIFO() iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range c.AllFIFO() {
			if !yield(v) {
				return
			}
		}
	}
}

// entriesOldestFirst returns the entries in insertion order, oldest first.
// Up to concurrency goroutines copy the shards, each under its lock, so the
// result does not show the whole cache at a single moment.
func (c *Cache[K, V]) entriesOldestFirst(concurrency int) []*entry[K, V] {
	shardEntries := make([][]*entry[K, V], shardsCount)

	var workers sync.WaitGroup

	for w := range concurrency {
		workers.Go(func() {
			for idx := w; idx < shardsCount; idx += concurrency {
				shard := &c.shards[idx]
				shard.mu.Lock()
				shardEntries[idx] = shard.appendLocked(make([]*entry[K, V], 0, shard.entryCount))
				shard.mu.Unlock()
			}
		})
	}

	workers.Wait()

	entries := slices.Concat(shardEntries...)

	// Sort references that hold the insertion number, so that comparisons do
	// not load the entries.
	type entryRef struct {
		seq   uint64
		entry *entry[K, V]
	}

	refs := make([]entryRef, len(entries))
	for i, e := range entries {
		refs[i] = entryRef{seq: e.seq, entry: e}
	}

	slices.SortFunc(refs, func(a, b entryRef) int { return cmp.Compare(a.seq, b.seq) })

	for i, r := range refs {
		entries[i] = r.entry
	}

	return entries
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

// weightOf returns the weight of (k, v), which is 0 in a cache made by New. It
// returns ErrInvalidWeight for a weight that no eviction can make room for.
func (c *Cache[K, V]) weightOf(k K, v V) (int, error) {
	if c.weigh == nil {
		return 0, nil
	}

	w := c.weigh(k, v)
	if w < 0 || w > c.maxWeight {
		return 0, fmt.Errorf("%w: got %d, max weight %d", ErrInvalidWeight, w, c.maxWeight)
	}

	return w, nil
}

// hasRoom reports whether the cache can take extra more weight and, if
// newEntry is true, one more entry.
func (c *Cache[K, V]) hasRoom(extra int, newEntry bool) bool {
	if newEntry && c.entryCount.Load() >= int64(c.maxEntries) {
		return false
	}

	return int64(extra) <= int64(c.maxWeight)-c.weight.Load()
}

// runInsert stores k unless op leaves an existing value in place. w is the
// weight of v.
func (c *Cache[K, V]) runInsert(op op, idx int, hash uint64, k K, v V, w int) (result[V], error) {
	c.orderMu.Lock()
	defer c.orderMu.Unlock()

	c.dropDeletedLocked()

	for {
		shard := &c.shards[idx]
		shard.mu.Lock()

		// need is the weight that storing v adds. Only Set replaces a value,
		// and a heavier one needs room first: eviction then skips keep, the
		// insertion number of k's entry, so that k stays in the cache, in its
		// place, while Set makes room. Insertion numbers start at 1, so a keep
		// of 0 means that k is absent, and skips nothing.
		need, keep := w, uint64(0)

		if t, pos, e := shard.findLocked(hash, k); pos >= 0 {
			need, keep = w-e.weight, e.seq

			if op != opSet || c.hasRoom(need, false) {
				result, err := c.handleExisting(op, shard, t, pos, e, v, w)
				shard.mu.Unlock()

				return result, err
			}
		} else if c.hasRoom(need, true) {
			result, err := c.handleInsert(op, idx, hash, k, v, w, shard)
			shard.mu.Unlock()

			return result, err
		}
		shard.mu.Unlock()

		// Deletes do not take orderMu, so since the check above they may
		// have removed every entry left to evict. That leaves room, and the
		// next pass stores v.
		if !c.evictOldestLocked(keep) && !c.hasRoom(need, keep == 0) {
			return result[V]{}, fmt.Errorf("%w: entry count=%d, max entries=%d", ErrEvictionFailed, c.entryCount.Load(), c.maxEntries)
		}
	}
}

func (c *Cache[K, V]) handleExisting(
	op op, shard *shard[K, V], t *table[K, V], pos int, e *entry[K, V], v V, w int,
) (result[V], error) {
	switch op {
	case opSet:
		shard.replaceLocked(t, pos, e, v, w)

		if w != e.weight {
			c.weight.Add(int64(w - e.weight))
		}

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

func (c *Cache[K, V]) handleInsert(
	op op, idx int, hash uint64, k K, v V, w int, shard *shard[K, V],
) (result[V], error) {
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
	shard.insertLocked(&entry[K, V]{Key: k, Value: v, seq: c.seq, hash: hash, weight: w})
	c.order.push(slot{shard: idx, hash: hash, seq: c.seq})
	c.entryCount.Add(1)

	if w != 0 {
		c.weight.Add(int64(w))
	}

	return res, nil
}

// evictOldestLocked evicts the oldest entry other than the one whose insertion
// number is keep, which keeps its place. It reports whether it evicted one.
func (c *Cache[K, V]) evictOldestLocked(keep uint64) bool {
	for c.order.len > 0 {
		slot := c.order.pop()

		shard := &c.shards[slot.shard]
		shard.mu.Lock()
		if t, pos := shard.findSeqLocked(slot.hash, slot.seq); pos >= 0 {
			if slot.seq == keep {
				shard.mu.Unlock()

				evicted := c.evictOldestLocked(0)
				c.order.pushFront(slot)

				return evicted
			}

			w := t.slots[pos].Load().weight
			shard.removeLocked(t, pos)
			shard.evictions++
			shard.mu.Unlock()
			c.entryCount.Add(-1)

			if w != 0 {
				c.weight.Add(-int64(w))
			}

			return true
		}
		shard.mu.Unlock()
	}

	return false
}

// dropDeletedLocked removes the slots of deleted entries from the order log,
// a few at a time. Delete leaves its slot behind, so without compaction Set
// and Delete churn grows the log without bound, and the next eviction has to
// skip every dead slot. A pass starts once dead slots outnumber live entries
// by minCompactSlots. If it runs to its end, it removes more slots than were
// live when it began, and the pushes it catches up with add at most
// 1/(compactStep-1) to the slots it checks. If evictions end it first, they
// have taken every slot it kept. Either way its cost stays O(1) per Delete or
// eviction. The pass is spread over the inserts that follow, compactStep
// slots each, because a shard lock and a table probe per slot would hold up
// every insert for time in proportion to the cache.
//
// Callers must not hold any shard lock.
func (c *Cache[K, V]) dropDeletedLocked() {
	if c.order.unchecked == 0 {
		// No pass is running.
		live := int(c.entryCount.Load())
		if c.order.len-live <= live+minCompactSlots {
			return
		}

		c.order.unchecked = c.order.len
	}

	c.order.compact(c.slotLive, compactStep)
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
//
// A compaction pass drops dead slots in place, a few per call to compact. From
// the oldest, the cells in use hold:
//
//	len-holes-unchecked  slots already checked and kept, in order
//	holes                stale copies that are no longer part of the queue
//	unchecked            slots not yet checked
//
// The queue is the kept slots followed by the unchecked ones, and a pass runs
// while unchecked is above 0. Slots pushed during a pass are unchecked. The
// counts start from the newest end, so pushFront leaves them alone, and pop
// changes them only when no kept slot is left; see pop for why that matters.
type slotRing struct {
	buf       []slot
	head      int // index in buf of the oldest slot
	len       int // number of cells in use, holes included
	holes     int // see above; both are 0 outside a pass
	unchecked int
}

// push adds s as the newest slot.
func (r *slotRing) push(s slot) {
	if r.len == len(r.buf) {
		r.grow()
	}

	r.buf[r.index(r.len)] = s
	r.len++

	if r.unchecked > 0 {
		r.unchecked++
	}
}

// pushFront adds s as the oldest slot. The ring must have a free cell, such as
// the one that pop frees.
func (r *slotRing) pushFront(s slot) {
	r.head = r.index(len(r.buf) - 1)
	r.buf[r.head] = s
	r.len++
}

// pop removes and returns the oldest slot. The ring must not be empty. pop
// runs on every eviction, so keep it cheap enough to inline.
func (r *slotRing) pop() slot {
	if r.len == r.holes+r.unchecked {
		// No kept slot is left, so the holes come first. Drop them, and
		// end the pass: every slot left is unchecked, which is where a new
		// pass would start.
		r.head = r.index(r.holes)
		r.len -= r.holes
		r.holes, r.unchecked = 0, 0
	}

	s := r.buf[r.head]
	r.head++
	if r.head == len(r.buf) {
		r.head = 0
	}
	r.len--

	return s
}

// compact checks up to n slots of the pass and keeps those for which live
// returns true.
func (r *slotRing) compact(live func(slot) bool, n int) {
	for ; n > 0 && r.unchecked > 0; n-- {
		i := r.len - r.unchecked
		if s := r.buf[r.index(i)]; live(s) {
			r.buf[r.index(i-r.holes)] = s
		} else {
			r.holes++
		}

		r.unchecked--
	}

	if r.unchecked == 0 {
		// The pass is over, and the holes are the newest cells.
		r.len -= r.holes
		r.holes = 0
	}
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
