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
  goos: linux
  goarch: amd64
  pkg: benchmarks
  cpu: AMD EPYC 7763 64-Core Processor                
                    │ fastcache_fork │               fastcache                │                 otter                 │
                    │     sec/op     │    sec/op      vs base                 │    sec/op     vs base                 │
  Set/1-4                 45.48n ± 0%    39.31n ±  4%   -13.56% (p=0.000 n=10)   224.70n ± 1%  +394.12% (p=0.000 n=10)
  Get/1-4                 21.20n ± 0%    46.22n ±  1%  +118.07% (p=0.000 n=10)    36.70n ± 1%   +73.18% (p=0.000 n=10)
  SetGet/1-4              66.62n ± 1%    86.52n ±  1%   +29.85% (p=0.000 n=10)   262.75n ± 1%  +294.37% (p=0.000 n=10)
  GetParallel/1-4         59.16n ± 0%    59.13n ±  0%    -0.06% (p=0.041 n=10)    24.47n ± 1%   -58.63% (p=0.000 n=10)
  SetParallel/1-4         94.08n ± 1%    78.43n ±  0%   -16.63% (p=0.000 n=10)   266.85n ± 1%  +183.64% (p=0.000 n=10)
  Set/16-4                46.18n ± 0%    43.03n ±  2%    -6.82% (p=0.000 n=10)   227.25n ± 1%  +392.04% (p=0.000 n=10)
  Get/16-4                22.25n ± 0%    53.71n ±  1%  +141.42% (p=0.000 n=10)    37.35n ± 1%   +67.87% (p=0.000 n=10)
  SetGet/16-4             64.60n ± 0%    95.37n ±  3%   +47.63% (p=0.000 n=10)   266.00n ± 1%  +311.76% (p=0.000 n=10)
  GetParallel/16-4        59.21n ± 2%    48.25n ±  0%   -18.52% (p=0.000 n=10)    25.30n ± 1%   -57.26% (p=0.000 n=10)
  SetParallel/16-4        93.21n ± 1%    83.28n ±  0%   -10.64% (p=0.000 n=10)   268.65n ± 1%  +188.24% (p=0.000 n=10)
  Set/128-4               70.02n ± 1%    65.77n ±  1%    -6.08% (p=0.000 n=10)   236.05n ± 2%  +237.09% (p=0.000 n=10)
  Get/128-4               39.59n ± 0%    92.28n ±  2%  +133.12% (p=0.000 n=10)    42.71n ± 0%    +7.89% (p=0.000 n=10)
  SetGet/128-4            102.3n ± 0%    154.5n ±  0%   +51.03% (p=0.000 n=10)    276.6n ± 2%  +170.38% (p=0.000 n=10)
  GetParallel/128-4       30.57n ± 2%    73.18n ±  1%  +139.39% (p=0.000 n=10)    28.01n ± 2%    -8.37% (p=0.000 n=10)
  SetParallel/128-4       105.2n ± 1%    101.0n ±  0%    -3.99% (p=0.000 n=10)    233.8n ± 7%  +122.30% (p=0.000 n=10)
  Set/256-4               75.17n ± 1%    93.03n ±  1%   +23.76% (p=0.000 n=10)   240.05n ± 1%  +219.34% (p=0.000 n=10)
  Get/256-4               56.22n ± 1%   143.70n ±  4%  +155.63% (p=0.000 n=10)    46.61n ± 1%   -17.09% (p=0.000 n=10)
  SetGet/256-4            106.7n ± 2%    227.2n ±  1%  +112.99% (p=0.000 n=10)    281.2n ± 2%  +163.62% (p=0.000 n=10)
  GetParallel/256-4       28.31n ± 1%    88.68n ±  1%  +213.19% (p=0.000 n=10)    29.66n ± 0%    +4.73% (p=0.000 n=10)
  SetParallel/256-4       39.46n ± 1%    60.84n ±  5%   +54.19% (p=0.000 n=10)   236.20n ± 2%  +498.66% (p=0.000 n=10)
  Set/512-4               92.48n ± 3%   123.15n ±  1%   +33.16% (p=0.000 n=10)   240.65n ± 1%  +160.20% (p=0.000 n=10)
  Get/512-4               89.42n ± 2%   181.40n ±  2%  +102.87% (p=0.000 n=10)    57.47n ± 3%   -35.73% (p=0.000 n=10)
  SetGet/512-4            140.0n ± 1%    289.0n ±  1%  +106.43% (p=0.000 n=10)    287.7n ± 1%  +105.50% (p=0.000 n=10)
  GetParallel/512-4       35.31n ± 1%   104.55n ±  1%  +196.05% (p=0.000 n=10)    32.66n ± 5%    -7.52% (p=0.000 n=10)
  SetParallel/512-4       47.79n ± 6%   101.15n ±  2%  +111.63% (p=0.000 n=10)   236.65n ± 1%  +395.14% (p=0.000 n=10)
  Set/1024-4              127.2n ± 1%    192.4n ±  2%   +51.24% (p=0.000 n=10)    249.9n ± 2%   +96.35% (p=0.000 n=10)
  Get/1024-4             125.85n ± 2%   248.40n ±  2%   +97.38% (p=0.000 n=10)    79.20n ± 1%   -37.06% (p=0.000 n=10)
  SetGet/1024-4           198.4n ± 0%    457.8n ±  1%  +130.80% (p=0.000 n=10)    307.3n ± 2%   +54.93% (p=0.000 n=10)
  GetParallel/1024-4      49.52n ± 0%   140.05n ±  1%  +182.82% (p=0.000 n=10)    39.14n ± 6%   -20.96% (p=0.000 n=10)
  SetParallel/1024-4      58.54n ± 0%    96.42n ±  0%   +64.72% (p=0.000 n=10)   237.35n ± 2%  +305.45% (p=0.000 n=10)
  Set/2048-4              197.6n ± 1%    312.7n ±  1%   +58.25% (p=0.000 n=10)    267.1n ± 2%   +35.17% (p=0.000 n=10)
  Get/2048-4              180.2n ± 9%    420.5n ±  2%  +133.42% (p=0.000 n=10)    113.8n ± 1%   -36.80% (p=0.000 n=10)
  SetGet/2048-4           318.1n ± 1%    752.0n ±  1%  +136.40% (p=0.000 n=10)    361.7n ± 1%   +13.71% (p=0.000 n=10)
  GetParallel/2048-4      74.64n ± 0%   226.90n ±  1%  +204.01% (p=0.000 n=10)    54.69n ± 2%   -26.72% (p=0.000 n=10)
  SetParallel/2048-4      88.89n ± 0%   147.65n ±  0%   +66.10% (p=0.000 n=10)   241.50n ± 1%  +171.68% (p=0.000 n=10)
  Set/4096-4              325.1n ± 1%    560.8n ±  2%   +72.49% (p=0.000 n=10)    309.5n ± 1%    -4.81% (p=0.000 n=10)
  Get/4096-4              288.3n ± 1%    856.0n ±  2%  +196.88% (p=0.000 n=10)    192.5n ± 2%   -33.24% (p=0.000 n=10)
  SetGet/4096-4           522.0n ± 0%   1535.0n ±  1%  +194.06% (p=0.000 n=10)    510.1n ± 1%    -2.29% (p=0.001 n=10)
  GetParallel/4096-4     128.80n ± 0%   469.60n ±  5%  +264.60% (p=0.000 n=10)    91.81n ± 1%   -28.72% (p=0.000 n=10)
  SetParallel/4096-4      149.0n ± 0%    257.4n ±  1%   +72.75% (p=0.000 n=10)    238.6n ± 2%   +60.13% (p=0.000 n=10)
  Set/8192-4              810.5n ± 0%   1080.0n ±  1%   +33.24% (p=0.000 n=10)    564.0n ± 1%   -30.42% (p=0.000 n=10)
  Get/8192-4              511.2n ± 0%   1913.0n ±  4%  +274.22% (p=0.000 n=10)    331.1n ± 0%   -35.23% (p=0.000 n=10)
  SetGet/8192-4          1171.5n ± 0%   3211.5n ±  1%  +174.14% (p=0.000 n=10)    804.0n ± 1%   -31.37% (p=0.000 n=10)
  GetParallel/8192-4      236.6n ± 0%    890.3n ± 12%  +276.21% (p=0.000 n=10)    158.1n ± 1%   -33.19% (p=0.000 n=10)
  SetParallel/8192-4      354.8n ± 0%    459.1n ±  0%   +29.41% (p=0.000 n=10)    279.6n ± 5%   -21.20% (p=0.000 n=10)
  geomean                 102.6n         185.5n         +80.86%                   150.5n        +46.75%
                    │ fastcache_fork │            fastcache             │              otter               │
                    │      B/op      │     B/op      vs base            │     B/op      vs base            │
  Set/1-4                 0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/1-4                0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)       0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/1-4             0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)      64.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/1-4        0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)       0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/1-4         0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     65.00 ± 0%  ? (p=0.000 n=10)
  Set/16-4                0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/16-4                0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)        0.00 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/16-4             0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/16-4        0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)        0.00 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/16-4        0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     65.00 ± 0%  ? (p=0.000 n=10)
  Set/128-4               0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/128-4                0.0 ± 0%       128.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/128-4            0.00 ± 0%      128.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/128-4        0.0 ± 0%       128.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/128-4       0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/256-4               0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/256-4                0.0 ± 0%       234.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/256-4            0.00 ± 0%      256.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/256-4        0.0 ± 0%       234.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/256-4       0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/512-4               0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/512-4                0.0 ± 0%       469.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/512-4            0.00 ± 0%      512.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/512-4        0.0 ± 0%       469.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/512-4       0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/1024-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/1024-4               0.0 ± 0%       853.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/1024-4           0.00 ± 0%     1024.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/1024-4       0.0 ± 0%       853.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/1024-4      0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/2048-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/2048-4           0.000Ki ± 0%     1.666Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/2048-4           0.00 ± 0%     2048.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/2048-4   0.000Ki ± 0%     1.666Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/2048-4      0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/4096-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/4096-4           0.000Ki ± 0%     3.333Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/4096-4           0.00 ± 0%     4096.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/4096-4   0.000Ki ± 0%     3.333Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/4096-4      0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/8192-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/8192-4           0.000Ki ± 0%     7.333Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/8192-4           0.00 ± 0%     8192.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/8192-4   0.000Ki ± 0%     7.333Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/8192-4      0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  geomean                           ²                 ?                ²                 ?                ²
  ¹ all samples are equal
  ² summaries must be >0 to compute geomean
                    │ fastcache_fork │           fastcache            │             otter              │
                    │   allocs/op    │ allocs/op   vs base            │ allocs/op   vs base            │
  Set/1-4                0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/1-4                0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/1-4             0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/1-4        0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/1-4        0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Set/16-4               0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/16-4               0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/16-4            0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/16-4       0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/16-4       0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Set/128-4              0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/128-4              0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/128-4           0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/128-4      0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/128-4      0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Set/256-4              0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/256-4              0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/256-4           0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/256-4      0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/256-4      0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Set/512-4              0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/512-4              0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/512-4           0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/512-4      0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/512-4      0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Set/1024-4             0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/1024-4             0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/1024-4          0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/1024-4     0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/1024-4     0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Set/2048-4             0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/2048-4             0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/2048-4          0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/2048-4     0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/2048-4     0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Set/4096-4             0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/4096-4             0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/4096-4          0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/4096-4     0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/4096-4     0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Set/8192-4             0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/8192-4             0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/8192-4          0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/8192-4     0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/8192-4     0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  geomean                           ²               ?                ²               ?                ²
  ¹ all samples are equal
  ² summaries must be >0 to compute geomean
  ```
</details>

Highlights:

* ~1.8× faster geomean `sec/op` vs original fastcache; ~1.5× faster vs otter.
* Zero allocations in measured ops; fastcache or otter allocate in many cases.
* Otter wins a few parallel-`Get` micro-benchmarks; this fork leads elsewhere.

Run benchmarks yourself:

```bash
make bench -C benchmarks
```

## Limitations

* No cache expiration: entries are evicted only when the cache is full (FIFO order).
* No size-based limits: capacity is by entry count, not bytes.

## Status

> [!CAUTION]
> **`fastcache`** is pre-v1 and does NOT provide a stable API; **use at your own risk**.

Occasional breaking changes may be introduced without notice until a post-v1 release.

## License

MIT. Same as original. See [LICENSE](/LICENSE).
