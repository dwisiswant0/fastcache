// Package fastcache provides a fast, generic, thread-safe in-memory cache
// with FIFO eviction.
//
// This is a fork of [VictoriaMetrics/fastcache] with a redesigned API using
// Go generics.
//
// # Architecture
//
// The cache uses a sharded design with 512 shards, each with its own lock.
// This reduces contention on multi-core CPUs. Each shard contains:
//
//   - A map[K]V for O(1) lookups.
//   - A ring buffer tracking insertion order for FIFO eviction.
//
// Keys are distributed across shards using [maphash.Comparable] for
// zero-allocation hashing of any comparable type.
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
// to/from [io.Writer]/[io.Reader] or files using [gob] encoding with [snappy]
// compression.
//
// # Thread Safety
//
// All [Cache] methods are safe for concurrent use by multiple goroutines.
// Iterator methods provide a snapshot view and do not block other operations.
//
// [VictoriaMetrics/fastcache]: https://github.com/VictoriaMetrics/fastcache.
package fastcache
