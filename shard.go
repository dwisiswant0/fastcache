package fastcache

import (
	"sync"
)

const shardsCount = 512

type shard[K comparable, V any] struct {
	mu sync.Mutex

	// stats (hits computed as getCalls - misses)
	getCalls  uint64
	setCalls  uint64
	misses    uint64
	deletes   uint64
	evictions uint64

	// entries maps a secure hash to one or more entries that share it.
	entries    map[uint64][]entry[K, V]
	entryCount int
}

// entry is used for serializing key-value pairs.
type entry[K comparable, V any] struct {
	Key   K
	Value V
}

func findEntry[K comparable, V any](bucket []entry[K, V], key K) int {
	for i := range bucket {
		if bucket[i].Key == key {
			return i
		}
	}

	return -1
}

func deleteEntry[K comparable, V any](bucket []entry[K, V], idx int) []entry[K, V] {
	last := len(bucket) - 1
	bucket[idx] = bucket[last]
	bucket[last] = entry[K, V]{}

	return bucket[:last]
}

func (s *shard[K, V]) set(c *Cache[K, V], idx int, hash uint64, k K, v V) error {
	s.mu.Lock()
	s.setCalls++

	// Update existing key - no count change
	bucket := s.entries[hash]
	if pos := findEntry(bucket, k); pos >= 0 {
		bucket[pos].Value = v
		s.mu.Unlock()

		return nil
	}
	s.mu.Unlock()

	_, err := c.runInsert(opSet, idx, hash, k, v)

	return err
}

func (s *shard[K, V]) get(hash uint64, k K) (V, bool) {
	s.mu.Lock()
	s.getCalls++
	bucket := s.entries[hash]
	if pos := findEntry(bucket, k); pos >= 0 {
		v := bucket[pos].Value
		s.mu.Unlock()

		return v, true
	}

	s.misses++
	s.mu.Unlock()
	// NOTE(dwisiswant0): hits = getCalls - misses (computed in [UpdateStats]).

	var zero V

	return zero, false
}

func (s *shard[K, V]) getOrSet(c *Cache[K, V], idx int, hash uint64, k K, v V) (V, bool, error) {
	s.mu.Lock()

	bucket := s.entries[hash]
	if pos := findEntry(bucket, k); pos >= 0 {
		s.getCalls++
		existing := bucket[pos].Value
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

	if findEntry(s.entries[hash], k) >= 0 {
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
	bucket := s.entries[hash]
	if pos := findEntry(bucket, k); pos >= 0 {
		bucket = deleteEntry(bucket, pos)
		if len(bucket) == 0 {
			delete(s.entries, hash)
		} else {
			s.entries[hash] = bucket
		}
		s.entryCount--
		c.entryCount.Add(-1)
	}
	s.mu.Unlock()
}

func (s *shard[K, V]) getAndDelete(c *Cache[K, V], hash uint64, k K) (V, bool) {
	s.mu.Lock()
	s.deletes++

	bucket := s.entries[hash]
	if pos := findEntry(bucket, k); pos >= 0 {
		v := bucket[pos].Value
		bucket = deleteEntry(bucket, pos)
		if len(bucket) == 0 {
			delete(s.entries, hash)
		} else {
			s.entries[hash] = bucket
		}
		s.entryCount--
		c.entryCount.Add(-1)
		s.mu.Unlock()

		return v, true
	}
	s.mu.Unlock()

	var zero V

	return zero, false
}

func (s *shard[K, V]) reset() {
	s.mu.Lock()
	s.entries = make(map[uint64][]entry[K, V])
	s.entryCount = 0
	s.getCalls = 0
	s.setCalls = 0
	s.misses = 0
	s.deletes = 0
	s.evictions = 0
	s.mu.Unlock()
}

func (s *shard[K, V]) rangeEntries(f func(k K, v V) bool) bool {
	s.mu.Lock()
	entries := make([]entry[K, V], 0, s.entryCount)
	for _, bucket := range s.entries {
		for _, e := range bucket {
			entries = append(entries, entry[K, V]{Key: e.Key, Value: e.Value})
		}
	}
	s.mu.Unlock()

	for _, entry := range entries {
		if !f(entry.Key, entry.Value) {
			return false
		}
	}

	return true
}

func (s *shard[K, V]) rangeKeys(f func(k K) bool) bool {
	s.mu.Lock()
	keys := make([]K, 0, s.entryCount)
	for _, bucket := range s.entries {
		for _, entry := range bucket {
			keys = append(keys, entry.Key)
		}
	}
	s.mu.Unlock()

	for _, k := range keys {
		if !f(k) {
			return false
		}
	}

	return true
}

func (s *shard[K, V]) rangeValues(f func(v V) bool) bool {
	s.mu.Lock()
	values := make([]V, 0, s.entryCount)
	for _, bucket := range s.entries {
		for _, entry := range bucket {
			values = append(values, entry.Value)
		}
	}
	s.mu.Unlock()

	for _, v := range values {
		if !f(v) {
			return false
		}
	}

	return true
}
