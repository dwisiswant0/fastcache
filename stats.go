package fastcache

import (
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
)

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
	for i := range c.gets {
		hits, misses := c.gets[i].hits.Load(), c.gets[i].misses.Load()
		s.GetCalls += hits + misses
		s.Misses += misses
	}

	for i := range c.shards {
		shard := &c.shards[i]
		shard.mu.Lock()
		s.SetCalls += shard.setCalls
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

// getCounter counts Get outcomes for the goroutines that share its stripe. It
// fills two cache lines (128 bytes covers the adjacent-line prefetch on amd64
// and the 128-byte lines of Apple silicon) so stripes never share one.
type getCounter struct {
	hits, misses atomic.Uint64
	_            [128 - 16]byte
}

// getStripe hands each P the stripe it last used, so concurrent Gets on
// different Ps add to different cache lines. A uint8 boxes in an interface
// without allocating.
var getStripe sync.Pool

// maxGetStripes is the most stripes a cache has; a stripe index is a uint8.
const maxGetStripes = 256

// newGetCounters returns one stripe per CPU, rounded up to a power of two so
// an index maps to a stripe with a mask. Cache.gets is read without a lock, so
// it is allocated once and never replaced.
func newGetCounters() []getCounter {
	n := 1
	for n < runtime.NumCPU() && n < maxGetStripes {
		n *= 2
	}

	return make([]getCounter, n)
}

// countGet counts one Get. A cache that no two Gets have counted at once, as
// in a serial program, counts on stripe 0 and skips the pool, which is
// slower than the single uncontended CAS. The first failed CAS shows
// contention, and the cache switches to the striped path for good.
func (c *Cache[K, V]) countGet(hit bool) {
	if !c.getsStriped.Load() {
		ctr := &c.gets[0].hits
		if !hit {
			ctr = &c.gets[0].misses
		}

		if old := ctr.Load(); ctr.CompareAndSwap(old, old+1) {
			return
		}

		c.getsStriped.Store(true)
	}

	i, ok := getStripe.Get().(uint8)
	if !ok {
		i = uint8(rand.Uint32())
	}

	for {
		g := &c.gets[int(i)&(len(c.gets)-1)]

		ctr := &g.hits
		if !hit {
			ctr = &g.misses
		}

		if old := ctr.Load(); ctr.CompareAndSwap(old, old+1) {
			break
		}

		// Another P shares this stripe; move this P to a random one.
		i = uint8(rand.Uint32())
	}

	getStripe.Put(i) //nolint:staticcheck // a uint8 boxes without allocating, so SA6002 does not apply
}
