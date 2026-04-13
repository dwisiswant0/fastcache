package fastcache

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

// UpdateStats adds cache stats to s.
//
// Call [Stats.Reset] before calling UpdateStats if s is re-used.
func (c *Cache[K, V]) UpdateStats(s *Stats) {
	for i := range c.shards {
		shard := &c.shards[i]
		shard.mu.Lock()
		s.GetCalls += shard.getCalls
		s.SetCalls += shard.setCalls
		s.Misses += shard.misses
		s.Deletes += shard.deletes
		s.Evictions += shard.evictions
		shard.mu.Unlock()
	}

	s.EntriesCount = uint64(c.entryCount.Load())
	s.Hits = s.GetCalls - s.Misses
	s.MaxEntries = uint64(c.maxEntries)
}

// Reset resets s, so it may be re-used again in [Cache.UpdateStats].
func (s *Stats) Reset() {
	*s = Stats{}
}
