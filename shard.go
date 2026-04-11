package fastcache

import (
	"sync"
	"sync/atomic"
)

const (
	shardsCount       = 512
	inlineShardEntries = 4
)

type inlineEntry[K comparable, V any] struct {
	key   K
	value V
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
	entries      map[K]V
	smallEntries []inlineEntry[K, V]

	// ring buffer for FIFO eviction order
	order    []keySlot[K] // circular buffer of keys
	writeIdx int          // next write pos
}

func (s *shard[K, V]) entryLen() int {
	if s.entries != nil {
		return len(s.entries)
	}

	return len(s.smallEntries)
}

func (s *shard[K, V]) inlineIndex(k K) int {
	for i := range s.smallEntries {
		if s.smallEntries[i].key == k {
			return i
		}
	}

	return -1
}

func (s *shard[K, V]) getEntry(k K) (V, bool) {
	if s.entries != nil {
		v, ok := s.entries[k]

		return v, ok
	}

	if idx := s.inlineIndex(k); idx >= 0 {
		return s.smallEntries[idx].value, true
	}

	var zero V

	return zero, false
}

func (s *shard[K, V]) setExisting(k K, v V) bool {
	if s.entries != nil {
		if _, exists := s.entries[k]; exists {
			s.entries[k] = v

			return true
		}

		return false
	}

	if idx := s.inlineIndex(k); idx >= 0 {
		s.smallEntries[idx].value = v

		return true
	}

	return false
}

func (s *shard[K, V]) promoteInlineEntries() {
	if s.entries != nil || len(s.smallEntries) == 0 {
		return
	}

	s.entries = make(map[K]V, len(s.smallEntries)+1)
	for _, entry := range s.smallEntries {
		s.entries[entry.key] = entry.value
	}
	s.smallEntries = s.smallEntries[:0]
}

func (s *shard[K, V]) insertEntry(k K, v V) {
	if s.entries != nil {
		s.entries[k] = v

		return
	}

	if len(s.smallEntries) < inlineShardEntries {
		s.smallEntries = append(s.smallEntries, inlineEntry[K, V]{key: k, value: v})

		return
	}

	s.promoteInlineEntries()
	s.entries[k] = v
}

func (s *shard[K, V]) deleteEntry(k K) (V, bool) {
	if s.entries != nil {
		v, exists := s.entries[k]
		if exists {
			delete(s.entries, k)
		}

		return v, exists
	}

	idx := s.inlineIndex(k)
	if idx < 0 {
		var zero V

		return zero, false
	}

	v := s.smallEntries[idx].value
	last := len(s.smallEntries) - 1
	s.smallEntries[idx] = s.smallEntries[last]
	s.smallEntries[last] = inlineEntry[K, V]{}
	s.smallEntries = s.smallEntries[:last]

	return v, true
}

func (s *shard[K, V]) set(c *Cache[K, V], k K, v V) {
	atomic.AddUint64(&s.setCalls, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update existing key - no count change
	if s.setExisting(k, v) {
		return
	}

	maxIter := len(s.order)
	for i := 0; i < maxIter && c.entryCount.Load() >= int64(c.maxEntries) && s.entryLen() > 0; i++ {
		slot := &s.order[s.writeIdx]
		if slot.valid {
			if _, exists := s.deleteEntry(slot.key); exists {
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
	s.insertEntry(k, v)
	c.entryCount.Add(1)
}

func (s *shard[K, V]) get(k K) (V, bool) {
	s.mu.RLock()
	atomic.AddUint64(&s.getCalls, 1)
	v, ok := s.getEntry(k)
	s.mu.RUnlock()

	if !ok {
		atomic.AddUint64(&s.misses, 1)
	}
	// NOTE(dwisiswant0): hits = getCalls - misses (computed in [UpdateStats]).

	return v, ok
}

func (s *shard[K, V]) getOrSet(c *Cache[K, V], k K, v V) (V, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.getEntry(k); ok {
		atomic.AddUint64(&s.getCalls, 1)

		return existing, true
	}

	atomic.AddUint64(&s.setCalls, 1)

	maxIter := len(s.order)
	for i := 0; i < maxIter && c.entryCount.Load() >= int64(c.maxEntries) && s.entryLen() > 0; i++ {
		slot := &s.order[s.writeIdx]
		if slot.valid {
			if _, exists := s.deleteEntry(slot.key); exists {
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
	s.insertEntry(k, v)
	c.entryCount.Add(1)

	return v, false
}

func (s *shard[K, V]) setIfAbsent(c *Cache[K, V], k K, v V) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.getEntry(k); exists {
		return false
	}

	atomic.AddUint64(&s.setCalls, 1)

	maxIter := len(s.order)
	for i := 0; i < maxIter && c.entryCount.Load() >= int64(c.maxEntries) && s.entryLen() > 0; i++ {
		slot := &s.order[s.writeIdx]
		if slot.valid {
			if _, exists := s.deleteEntry(slot.key); exists {
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
	s.insertEntry(k, v)
	c.entryCount.Add(1)

	return true
}

func (s *shard[K, V]) delete(c *Cache[K, V], k K) {
	atomic.AddUint64(&s.deletes, 1)

	s.mu.Lock()
	// NOTE(dwisiswant0): we don't remove from the ring buffer here.
	// The slot will be marked invalid when we encounter it during eviction.
	// This is O(1) delete vs O(n) if we searched the ring buffer btw.
	if _, exists := s.deleteEntry(k); exists {
		c.entryCount.Add(-1)
	}
	s.mu.Unlock()
}

func (s *shard[K, V]) getAndDelete(c *Cache[K, V], k K) (V, bool) {
	atomic.AddUint64(&s.deletes, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	v, exists := s.deleteEntry(k)
	if exists {
		c.entryCount.Add(-1)
	}

	return v, exists
}

func (s *shard[K, V]) reset() {
	s.mu.Lock()
	s.entries = nil
	if cap(s.smallEntries) > 0 {
		s.smallEntries = s.smallEntries[:0]
	}
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

func (s *shard[K, V]) rangeEntries(f func(k K, v V) bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.entries != nil {
		for k, v := range s.entries {
			if !f(k, v) {
				return false
			}
		}

		return true
	}

	for _, entry := range s.smallEntries {
		if !f(entry.key, entry.value) {
			return false
		}
	}

	return true
}

func (s *shard[K, V]) rangeKeys(f func(k K) bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.entries != nil {
		for k := range s.entries {
			if !f(k) {
				return false
			}
		}

		return true
	}

	for _, entry := range s.smallEntries {
		if !f(entry.key) {
			return false
		}
	}

	return true
}

func (s *shard[K, V]) rangeValues(f func(v V) bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.entries != nil {
		for _, v := range s.entries {
			if !f(v) {
				return false
			}
		}

		return true
	}

	for _, entry := range s.smallEntries {
		if !f(entry.value) {
			return false
		}
	}

	return true
}
