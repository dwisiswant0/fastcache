# fastcache

[![Go Reference](https://pkg.go.dev/badge/go.dw1.io/fastcache.svg)](https://pkg.go.dev/go.dw1.io/fastcache)

A fast, generic, thread-safe in-memory cache for Go with FIFO eviction.

> [!NOTE]
> This is a fork of [VictoriaMetrics/fastcache](https://github.com/VictoriaMetrics/fastcache) with a redesigned API using Go generics.

## Features

* **Generic**: Type-safe API.
* **Zero allocations**: No allocations on `Get` ops.
* **Thread-safe**: Concurrent goroutines may read and write into a single cache instance.
* **FIFO eviction**: Oldest entries are evicted first when the cache is full.
* **Simple API**: Just `New`, `Get`, `Set`, `Delete`, `Has`, `Len`, `Range`, `Reset`.
* **Persistence**: Cache can be saved to file and loaded from file.

## Install

```bash
go get go.dw1.io/fastcache
```

## Usage

```go
package main

import (
    "fmt"

    "go.dw1.io/fastcache"
)

func main() {
    // create a cache with max 10000 entries
    c := fastcache.New[string, int](10000)

    // set values
    c.Set("foo", 123)
    c.Set("bar", 456)

    // get values
    if v, ok := c.Get("foo"); ok {
        fmt.Println("foo =", v) // foo = 123
    }

    // check existence
    if c.Has("bar") {
        fmt.Println("bar exists")
    }

    // delete
    c.Delete("foo")

    // iterate
    c.Range(func(k string, v int) bool {
        fmt.Printf("%s: %d\n", k, v)

        return true // continue iteration
    })

    // Get stats
    var stats fastcache.Stats
    c.UpdateStats(&stats)
    fmt.Printf("Hits: %d, Misses: %d\n", stats.Hits, stats.Misses)

    // save to file
    if err := c.SaveToFile("/tmp/cache.bin"); err != nil {
        panic(err)
    }

    // load from file
    c2, err := fastcache.LoadFromFile[string, int]("/tmp/cache.bin")
    if err != nil {
        panic(err)
    }
    defer c2.Reset()
}
```

## Architecture

The cache uses a sharded design for concurrent scalability:

* **512 shards**: Each with its own lock, reducing contention on multi-core CPUs.
* **Generic map storage**: `map[K]V` per shard for O(1) lookups.
* **Ring buffer for FIFO**: Circular buffer tracks insertion order for eviction.
* **maphash.Comparable**: Zero-allocation hashing for any comparable key type.

## Differences from Original

| Aspect | Original fastcache | This fork |
|--------|-------------------|-----------|
| **API** | `[]byte` keys/values | Generic `[K, V]` |
| **Capacity** | Bytes-based | Entry-count based |
| **Storage** | Ring buffer of bytes | `map[K]V` |
| **Eviction** | FIFO (by byte position) | FIFO (by insertion order) |
| **Allocations** | 1 alloc/Get | Zero |

## Benchmarks

Compared against the original [VictoriaMetrics/fastcache](https://github.com/VictoriaMetrics/fastcache) and [maypok86/otter](https://github.com/maypok86/otter):

<details open>
  <summary><code>benchstat</code></summary>

  ```
  TBD
  ```
</details>

Run benchmarks yourself:

```bash
make bench -C benchmarks
```

## Limitations

* No cache expiration: entries are evicted only when the cache is full (FIFO order).
* No size-based limits: capacity is by entry count, not bytes.

## License

MIT. Same as original. See [LICENSE](/LICENSE).
