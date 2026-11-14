package fastcache

import (
	"sync"
	"sync/atomic"
)

const shardsCount = 512

// shardBits is the number of low hash bits that pick the shard. A table probes
// from the bits above them: every key in a shard has the same low bits, so
// those would send all of them to one slot.
const shardBits = 9

// minTableSlots is the size of the first table a shard allocates.
const minTableSlots = 8

type shard[K comparable, V any] struct {
	mu sync.Mutex

	// stats guarded by mu. Get counts hits and misses in Cache.gets, since a
	// counter in the shard would make every Get write the shard's cache line.
	setCalls  uint64
	deletes   uint64
	evictions uint64

	// table is read without mu. Only holders of mu store it or its slots.
	table atomic.Pointer[table[K, V]]

	// moves counts the deletes that have shifted entries in table, and is
	// odd while one runs. A probe that misses checks it, because a shift can
	// carry the key behind the probe.
	moves atomic.Uint64

	entryCount int // live entries, guarded by mu
}

// table is an open-addressing hash table with linear probing. Get probes it
// with atomic loads only, so it never waits for a writer and never writes a
// cache line that other readers share. That rests on these invariants:
//
//   - An entry is immutable once published. Updating a value publishes a copy,
//     so a reader never sees a Key with another entry's Value.
//   - Only holders of the shard's mu write the table that is current. A
//     table that is not current is no longer written at all.
//   - Insertion only fills nil slots. Unless a delete runs meanwhile, a probe
//     that reaches nil has passed every slot its key could occupy.
//   - A delete moves later entries back into the hole it leaves, so a key may
//     pass a probe going the other way. The shard's moves counter tells the
//     probe to look again.
type table[K comparable, V any] struct {
	slots []atomic.Pointer[entry[K, V]]
}

// entry is a stored key-value pair. Key and Value are exported for gob
// serialization; seq is the insertion number shared with the entry's slot and
// hash is the key's hash. Neither is serialized.
type entry[K comparable, V any] struct {
	Key   K
	Value V
	seq   uint64
	hash  uint64
}

// home returns the slot where the probe for hash starts.
func (t *table[K, V]) home(hash uint64) uint64 {
	return (hash >> shardBits) & uint64(len(t.slots)-1)
}

// find returns the position and entry of k in t, or -1 and nil.
func (t *table[K, V]) find(hash uint64, k K) (int, *entry[K, V]) {
	mask := uint64(len(t.slots) - 1)
	for i := t.home(hash); ; i = (i + 1) & mask {
		e := t.slots[i].Load()
		if e == nil {
			return -1, nil
		}

		// Compare hashes first so that a probe past other keys does not
		// compare keys, which may be costly.
		if e.hash == hash && e.Key == k {
			return int(i), e
		}
	}
}

// findSeq returns the position of the entry with the given insertion number,
// or -1.
func (t *table[K, V]) findSeq(hash, seq uint64) int {
	mask := uint64(len(t.slots) - 1)
	for i := t.home(hash); ; i = (i + 1) & mask {
		e := t.slots[i].Load()
		if e == nil {
			return -1
		}

		if e.hash == hash && e.seq == seq {
			return int(i)
		}
	}
}

// put stores e in the first nil slot of its probe.
func (t *table[K, V]) put(e *entry[K, V]) {
	mask := uint64(len(t.slots) - 1)

	i := t.home(e.hash)
	for t.slots[i].Load() != nil {
		i = (i + 1) & mask
	}

	t.slots[i].Store(e)
}

// lookup returns the entry for k. It probes without locking: a hit needs no
// check, since the entry it returns was the key's entry when the probe loaded
// it. A miss is trusted only if no delete moved entries during the probe.
// Otherwise lookup probes again under the lock the delete holds, instead of
// spinning until the delete is done.
func (s *shard[K, V]) lookup(hash uint64, k K) *entry[K, V] {
	moves := s.moves.Load()

	t := s.table.Load()
	if t == nil {
		return nil
	}

	if _, e := t.find(hash, k); e != nil {
		return e
	}

	if moves%2 == 0 && s.moves.Load() == moves {
		return nil
	}

	s.mu.Lock()
	_, _, e := s.findLocked(hash, k)
	s.mu.Unlock()

	return e
}

// findLocked returns the current table and the position of k in it, or -1.
func (s *shard[K, V]) findLocked(hash uint64, k K) (*table[K, V], int, *entry[K, V]) {
	t := s.table.Load()
	if t == nil {
		return nil, -1, nil
	}

	pos, e := t.find(hash, k)

	return t, pos, e
}

// findSeqLocked returns the current table and the position in it of the entry
// with the given insertion number, or -1.
func (s *shard[K, V]) findSeqLocked(hash, seq uint64) (*table[K, V], int) {
	t := s.table.Load()
	if t == nil {
		return nil, -1
	}

	return t, t.findSeq(hash, seq)
}

// insertLocked publishes e, which must hold a key absent from the shard. The
// table stays at most half full, so every probe reaches a nil slot.
func (s *shard[K, V]) insertLocked(e *entry[K, V]) {
	t := s.table.Load()
	if t == nil || 2*(s.entryCount+1) > len(t.slots) {
		t = s.growLocked()
	}

	t.put(e)

	s.entryCount++
}

// growLocked publishes a table twice the size of the current one, holding the
// same entries, and returns it. A reader that loaded the old table finishes
// its probe there and sees the shard as it was before this call.
func (s *shard[K, V]) growLocked() *table[K, V] {
	old := s.table.Load()

	size := minTableSlots
	if old != nil {
		size = 2 * len(old.slots)
	}

	t := &table[K, V]{slots: make([]atomic.Pointer[entry[K, V]], size)}

	if old != nil {
		for i := range old.slots {
			if e := old.slots[i].Load(); e != nil {
				t.put(e)
			}
		}
	}

	s.table.Store(t)

	return t
}

// replaceLocked publishes v as the value of the entry e at pos. It publishes
// a copy because a published entry is immutable.
func (s *shard[K, V]) replaceLocked(t *table[K, V], pos int, e *entry[K, V], v V) {
	t.slots[pos].Store(&entry[K, V]{Key: e.Key, Value: v, seq: e.seq, hash: e.hash})
}

// removeLocked deletes the entry at pos and moves later entries of its run
// back into the hole, so the run stays unbroken without a tombstone.
// Tombstones would pile up under insert and delete churn until the table had
// to be rebuilt whole: a pause in proportion to the shard, which grows with
// the cache.
func (s *shard[K, V]) removeLocked(t *table[K, V], pos int) {
	s.moves.Add(1)

	mask := uint64(len(t.slots) - 1)
	hole := uint64(pos)

	for i := (hole + 1) & mask; ; i = (i + 1) & mask {
		e := t.slots[i].Load()
		if e == nil {
			break
		}

		// e may fill the hole only if its probe, which runs from its home
		// slot to i, passes the hole.
		if (i-t.home(e.hash))&mask >= (i-hole)&mask {
			t.slots[hole].Store(e)
			hole = i
		}
	}

	t.slots[hole].Store(nil)
	s.moves.Add(1)

	s.entryCount--
}

// appendLocked appends the live entries to dst. Entries are immutable, so the
// caller may read them after unlocking.
func (s *shard[K, V]) appendLocked(dst []*entry[K, V]) []*entry[K, V] {
	t := s.table.Load()
	if t == nil {
		return dst
	}

	for i := range t.slots {
		if e := t.slots[i].Load(); e != nil {
			dst = append(dst, e)
		}
	}

	return dst
}

func (s *shard[K, V]) set(c *Cache[K, V], idx int, hash uint64, k K, v V) error {
	s.mu.Lock()
	s.setCalls++

	// Update existing key - no count change
	if t, pos, e := s.findLocked(hash, k); pos >= 0 {
		s.replaceLocked(t, pos, e, v)
		s.mu.Unlock()

		return nil
	}
	s.mu.Unlock()

	_, err := c.runInsert(opSet, idx, hash, k, v)

	return err
}

func (s *shard[K, V]) getOrSet(c *Cache[K, V], idx int, hash uint64, k K, v V) (V, bool, error) {
	s.mu.Lock()

	if _, pos, e := s.findLocked(hash, k); pos >= 0 {
		c.countGet(true)

		existing := e.Value
		s.mu.Unlock()

		return existing, true, nil
	}
	s.mu.Unlock()

	result, err := c.runInsert(opGetOrSet, idx, hash, k, v)
	if err != nil {
		var zero V

		return zero, false, err
	}

	return result.value, result.loaded, nil
}

func (s *shard[K, V]) setIfAbsent(c *Cache[K, V], idx int, hash uint64, k K, v V) (bool, error) {
	s.mu.Lock()

	if _, pos, _ := s.findLocked(hash, k); pos >= 0 {
		s.mu.Unlock()

		return false, nil
	}
	s.mu.Unlock()

	result, err := c.runInsert(opSetIfAbsent, idx, hash, k, v)
	if err != nil {
		return false, err
	}

	return result.stored, nil
}

func (s *shard[K, V]) delete(c *Cache[K, V], hash uint64, k K) {
	s.mu.Lock()
	s.deletes++

	if t, pos, _ := s.findLocked(hash, k); pos >= 0 {
		s.removeLocked(t, pos)
		c.entryCount.Add(-1)
	}
	s.mu.Unlock()
}

func (s *shard[K, V]) getAndDelete(c *Cache[K, V], hash uint64, k K) (V, bool) {
	s.mu.Lock()
	s.deletes++

	if t, pos, e := s.findLocked(hash, k); pos >= 0 {
		s.removeLocked(t, pos)
		c.entryCount.Add(-1)
		s.mu.Unlock()

		return e.Value, true
	}
	s.mu.Unlock()

	var zero V

	return zero, false
}

func (s *shard[K, V]) reset() {
	s.mu.Lock()
	s.table.Store(nil)
	s.entryCount = 0
	s.setCalls = 0
	s.deletes = 0
	s.evictions = 0
	s.mu.Unlock()
}

func (s *shard[K, V]) rangeEntries(f func(k K, v V) bool) bool {
	s.mu.Lock()
	entries := s.appendLocked(make([]*entry[K, V], 0, s.entryCount))
	s.mu.Unlock()

	for _, e := range entries {
		if !f(e.Key, e.Value) {
			return false
		}
	}

	return true
}

func (s *shard[K, V]) rangeKeys(f func(k K) bool) bool {
	return s.rangeEntries(func(k K, _ V) bool { return f(k) })
}

func (s *shard[K, V]) rangeValues(f func(v V) bool) bool {
	return s.rangeEntries(func(_ K, v V) bool { return f(v) })
}
