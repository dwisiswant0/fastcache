package fastcache

import (
	"sync"
	"sync/atomic"
)

const shardsCount = 512

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

func (s *shard[K, V]) getOrSet(c *Cache[K, V], k K, v V) (V, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.entries[k]; ok {
		atomic.AddUint64(&s.getCalls, 1)

		return existing, true
	}

	atomic.AddUint64(&s.setCalls, 1)

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

	return v, false
}

func (s *shard[K, V]) setIfAbsent(c *Cache[K, V], k K, v V) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[k]; exists {
		return false
	}

	atomic.AddUint64(&s.setCalls, 1)

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

	return true
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

func (s *shard[K, V]) getAndDelete(c *Cache[K, V], k K) (V, bool) {
	atomic.AddUint64(&s.deletes, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	v, exists := s.entries[k]
	if exists {
		delete(s.entries, k)
		c.entryCount.Add(-1)
	}

	return v, exists
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

func (s *shard[K, V]) rangeKeys(f func(k K) bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for k := range s.entries {
		if !f(k) {
			return false
		}
	}

	return true
}

func (s *shard[K, V]) rangeValues(f func(v V) bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, v := range s.entries {
		if !f(v) {
			return false
		}
	}

	return true
}
