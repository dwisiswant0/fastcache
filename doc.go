// Package fastcache provides a fast, generic, thread-safe in-memory cache
// with FIFO eviction.
//
// This is a fork of [VictoriaMetrics/fastcache] with a redesigned API using
// Go generics.
//
// # Architecture
//
// The cache uses a sharded design with 512 shards, each with its own lock.
// This reduces contention on multi-core CPUs. Each shard stores its entries
// in a hash map keyed by the rapidhash value of the key, for O(1) lookups.
//
// A single FIFO queue shared by all shards tracks insertion order, so the
// capacity limit and eviction order apply to the whole cache. Adding a new key
// also locks this queue, so new keys are inserted one at a time across the
// whole cache. Reads, updates of existing keys, and deletes lock only the
// key's shard.
//
// Keys are distributed across shards using rapidhash-based shard hashing.
//
// # Eviction
//
// When the cache reaches capacity, the oldest entries are evicted first
// (FIFO - First In, First Out). There is no time-based expiration; entries
// are only evicted when space is needed for new entries.
//
// # Iteration
//
// The cache provides Go 1.23+ iterators for range-based iteration:
//
//   - [Cache.All] - iterate over key-value pairs.
//   - [Cache.Keys] - iterate over keys only.
//   - [Cache.Values] - iterate over values only.
//
// # Atomic Operations
//
// The cache provides atomic compound operations:
//
//   - [Cache.GetOrSet] - get existing value or store new one.
//   - [Cache.GetAndDelete] - atomically get and remove a value.
//   - [Cache.SetIfAbsent] - store only if key doesn't exist.
//
// # Persistence
//
// The cache can be saved (with [Cache.SaveTo], [Cache.SaveToFile], and
// [Cache.SaveToFileConcurrent]) and loaded (from [LoadFrom] and [LoadFromFile])
// to/from [io.Writer]/[io.Reader] or files using [gob] encoding with [minlz]
// compression.
//
// # Thread Safety
//
// All [Cache] methods are safe for concurrent use by multiple goroutines.
// Iterator methods copy one shard at a time while holding its lock, then
// yield entries from the copy. Other operations on a shard wait only during
// its copy, and the entries yielded do not reflect a single moment for the
// whole cache.
//
// [VictoriaMetrics/fastcache]: https://github.com/VictoriaMetrics/fastcache.
package fastcache
