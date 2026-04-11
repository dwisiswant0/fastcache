# fastcache

[![Go Reference](https://pkg.go.dev/badge/go.dw1.io/fastcache.svg)](https://pkg.go.dev/go.dw1.io/fastcache)

A fast, generic, thread-safe cache for Go.

> [!NOTE]
> This is a fork of [VictoriaMetrics/fastcache](https://github.com/VictoriaMetrics/fastcache) with a redesigned API using Go generics.

## Features

* **Generic**: Type-safe API.
* **Zero allocations**: No allocations on `Get` ops.
* **Thread-safe**: Concurrent goroutines may read and write into a single cache instance.
* **FIFO eviction**: Oldest entries are evicted first when the cache is full.
* **Iterators**: Go 1.23+ range-over-func support with `All()`, `Keys()`, `Values()`.
* **Atomic operations**: `GetOrSet`, `GetAndDelete`, `SetIfAbsent` for lock-free patterns.
* **Persistence**: Cache can be saved to file and loaded from file.
* **Simple API**: See [Go reference](https://pkg.go.dev/go.dw1.io/fastcache).

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
    for k, v := range c.All() {
        fmt.Printf("%s: %d\n", k, v)
    }

    // iterate keys only
    for k := range c.Keys() {
        fmt.Println(k)
    }

    // atomic get-or-set
    actual, loaded := c.GetOrSet("baz", 789)
    fmt.Printf("value=%d, existed=%v\n", actual, loaded)

    // atomic set-if-absent
    if c.SetIfAbsent("qux", 101112) {
        fmt.Println("qux was set")
    }

    // atomic get-and-delete
    if v, ok := c.GetAndDelete("bar"); ok {
        fmt.Println("deleted bar:", v)
    }

    // get stats
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
                    │ fastcache_fork │               fastcache                │                 otter                  │
                    │     sec/op     │    sec/op     vs base                  │    sec/op      vs base                 │
  Set/1-4                 25.95n ± 0%    41.48n ± 1%    +59.87% (p=0.000 n=10)   223.65n ±  1%  +761.85% (p=0.000 n=10)
  Get/1-4                 17.81n ± 0%    46.87n ± 0%   +163.06% (p=0.000 n=10)    33.67n ±  0%   +89.03% (p=0.000 n=10)
  SetGet/1-4              42.17n ± 0%    88.21n ± 1%   +109.18% (p=0.000 n=10)   280.65n ±  0%  +565.52% (p=0.000 n=10)
  GetParallel/1-4         52.64n ± 0%    60.42n ± 0%    +14.78% (p=0.000 n=10)    23.74n ±  0%   -54.90% (p=0.000 n=10)
  SetParallel/1-4         61.69n ± 4%    76.70n ± 1%    +24.32% (p=0.000 n=10)   252.05n ±  0%  +308.58% (p=0.000 n=10)
  Set/16-4                28.29n ± 0%    42.31n ± 1%    +49.54% (p=0.000 n=10)   228.20n ±  1%  +706.65% (p=0.000 n=10)
  Get/16-4                20.47n ± 0%    56.78n ± 1%   +177.38% (p=0.000 n=10)    35.29n ±  1%   +72.40% (p=0.000 n=10)
  SetGet/16-4             46.27n ± 0%    95.00n ± 1%   +105.31% (p=0.000 n=10)   285.95n ±  1%  +518.00% (p=0.000 n=10)
  GetParallel/16-4        52.65n ± 0%    47.04n ± 0%    -10.65% (p=0.000 n=10)    24.84n ± 14%   -52.81% (p=0.000 n=10)
  SetParallel/16-4        65.27n ± 2%    80.39n ± 1%    +23.17% (p=0.000 n=10)   255.80n ±  0%  +291.91% (p=0.000 n=10)
  Set/128-4               65.08n ± 0%    63.05n ± 1%     -3.13% (p=0.000 n=10)   243.10n ±  1%  +273.51% (p=0.000 n=10)
  Get/128-4               56.29n ± 0%    97.37n ± 1%    +72.99% (p=0.000 n=10)    38.10n ±  0%   -32.31% (p=0.000 n=10)
  SetGet/128-4            119.7n ± 1%    156.9n ± 2%    +31.17% (p=0.000 n=10)    301.3n ±  1%  +151.82% (p=0.000 n=10)
  GetParallel/128-4       34.86n ± 0%    79.39n ± 1%   +127.74% (p=0.000 n=10)    28.05n ± 12%   -19.54% (p=0.000 n=10)
  SetParallel/128-4       74.64n ± 1%    98.85n ± 1%    +32.44% (p=0.000 n=10)   229.25n ±  5%  +207.14% (p=0.000 n=10)
  Set/256-4               63.60n ± 0%    92.17n ± 0%    +44.94% (p=0.000 n=10)   245.25n ±  0%  +285.64% (p=0.000 n=10)
  Get/256-4               55.05n ± 1%   166.30n ± 3%   +202.06% (p=0.000 n=10)    41.98n ±  1%   -23.74% (p=0.000 n=10)
  SetGet/256-4            116.7n ± 0%    240.6n ± 1%   +106.21% (p=0.000 n=10)    305.6n ±  1%  +161.83% (p=0.000 n=10)
  GetParallel/256-4       33.83n ± 1%   100.20n ± 2%   +196.19% (p=0.000 n=10)    27.57n ± 13%   -18.50% (p=0.000 n=10)
  SetParallel/256-4       39.52n ± 0%    64.17n ± 5%    +62.39% (p=0.000 n=10)   230.50n ±  1%  +483.25% (p=0.000 n=10)
  Set/512-4               64.37n ± 0%   122.90n ± 1%    +90.93% (p=0.000 n=10)   249.90n ±  0%  +288.22% (p=0.000 n=10)
  Get/512-4               55.17n ± 0%   198.75n ± 1%   +260.25% (p=0.000 n=10)    51.42n ±  1%    -6.80% (p=0.000 n=10)
  SetGet/512-4            116.9n ± 0%    308.9n ± 1%   +164.17% (p=0.000 n=10)    315.7n ±  1%  +169.94% (p=0.000 n=10)
  GetParallel/512-4       34.07n ± 1%   120.00n ± 4%   +252.16% (p=0.000 n=10)    32.72n ±  8%    -3.98% (p=0.041 n=10)
  SetParallel/512-4       40.13n ± 1%   108.25n ± 1%   +169.71% (p=0.000 n=10)   234.30n ±  3%  +483.78% (p=0.000 n=10)
  Set/1024-4              65.22n ± 0%   190.65n ± 0%   +192.32% (p=0.000 n=10)   263.10n ±  1%  +303.40% (p=0.000 n=10)
  Get/1024-4              56.22n ± 0%   258.55n ± 3%   +359.89% (p=0.000 n=10)    76.93n ±  2%   +36.84% (p=0.000 n=10)
  SetGet/1024-4           117.5n ± 1%    460.5n ± 1%   +291.87% (p=0.000 n=10)    331.2n ±  1%  +181.91% (p=0.000 n=10)
  GetParallel/1024-4      33.90n ± 0%   154.75n ± 2%   +356.49% (p=0.000 n=10)    41.05n ±  7%   +21.09% (p=0.000 n=10)
  SetParallel/1024-4      40.06n ± 0%    98.45n ± 1%   +145.73% (p=0.000 n=10)   234.70n ±  1%  +485.80% (p=0.000 n=10)
  Set/2048-4              67.11n ± 1%   314.25n ± 1%   +368.26% (p=0.000 n=10)   288.00n ±  0%  +329.15% (p=0.000 n=10)
  Get/2048-4              58.73n ± 1%   485.80n ± 1%   +727.25% (p=0.000 n=10)   121.95n ±  3%  +107.66% (p=0.000 n=10)
  SetGet/2048-4           119.8n ± 1%    811.5n ± 1%   +577.42% (p=0.000 n=10)    376.9n ±  2%  +214.61% (p=0.000 n=10)
  GetParallel/2048-4      33.90n ± 0%   267.80n ± 1%   +689.97% (p=0.000 n=10)    58.31n ±  4%   +72.02% (p=0.000 n=10)
  SetParallel/2048-4      40.18n ± 0%   150.90n ± 0%   +275.56% (p=0.000 n=10)   239.95n ±  1%  +497.19% (p=0.000 n=10)
  Set/4096-4              73.43n ± 1%   576.25n ± 1%   +684.76% (p=0.000 n=10)   321.40n ±  1%  +337.70% (p=0.000 n=10)
  Get/4096-4              65.52n ± 1%   934.85n ± 1%  +1326.93% (p=0.000 n=10)   202.65n ±  1%  +209.32% (p=0.000 n=10)
  SetGet/4096-4           125.2n ± 1%   1642.0n ± 1%  +1210.98% (p=0.000 n=10)    482.1n ±  1%  +284.95% (p=0.000 n=10)
  GetParallel/4096-4      35.51n ± 0%   509.65n ± 1%  +1335.23% (p=0.000 n=10)    96.88n ±  3%  +172.82% (p=0.000 n=10)
  SetParallel/4096-4      40.40n ± 0%   260.50n ± 1%   +544.72% (p=0.000 n=10)   231.80n ±  4%  +473.69% (p=0.000 n=10)
  Set/8192-4              130.1n ± 2%   1091.5n ± 1%   +738.97% (p=0.000 n=10)    556.0n ±  0%  +327.36% (p=0.000 n=10)
  Get/8192-4              125.4n ± 2%   1997.0n ± 5%  +1492.50% (p=0.000 n=10)    356.1n ±  1%  +184.01% (p=0.000 n=10)
  SetGet/8192-4           236.5n ± 1%   3291.0n ± 1%  +1291.54% (p=0.000 n=10)    792.1n ±  1%  +234.95% (p=0.000 n=10)
  GetParallel/8192-4      58.91n ± 1%   980.30n ± 6%  +1564.21% (p=0.000 n=10)   166.50n ±  0%  +182.66% (p=0.000 n=10)
  SetParallel/8192-4      69.75n ± 3%   459.60n ± 0%   +558.88% (p=0.000 n=10)   297.50n ± 21%  +326.49% (p=0.000 n=10)
  geomean                 57.04n         193.5n        +239.23%                   151.6n        +165.80%
                    │ fastcache_fork │            fastcache             │              otter               │
                    │      B/op      │     B/op      vs base            │     B/op      vs base            │
  Set/1-4                 0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/1-4                0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)       0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/1-4             0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)      64.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/1-4        0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)       0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/1-4         0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/16-4                0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/16-4                0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)        0.00 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/16-4             0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/16-4        0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)        0.00 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/16-4        0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     65.00 ± 2%  ? (p=0.000 n=10)
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

* ~3.4x faster geomean `sec/op` vs original fastcache; ~2.7x faster vs otter.
* Zero allocations in measured ops; original fastcache still allocates across many read and mixed workloads, and otter allocates on writes.
* Otter still wins a few small-key read-heavy micro-benchmarks, mainly `GetParallel` up to 512-byte keys; this fork leads on the rest and pulls away hard on large keys.

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
