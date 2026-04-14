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
    c, err := fastcache.New[string, int](10000)
    if err != nil {
        fmt.Println("create cache:", err)
        return
    }

    // set values
    if err := c.Set("foo", 123); err != nil {
        fmt.Println("set foo:", err)
        return
    }
    if err := c.Set("bar", 456); err != nil {
        fmt.Println("set bar:", err)
        return
    }

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
    actual, loaded, err := c.GetOrSet("baz", 789)
    if err != nil {
        fmt.Println("get or set baz:", err)
        return
    }
    fmt.Printf("value=%d, existed=%v\n", actual, loaded)

    // atomic set-if-absent
    stored, err := c.SetIfAbsent("qux", 101112)
    if err != nil {
        fmt.Println("set if absent qux:", err)
        return
    }
    if stored {
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
	    fmt.Println("save cache:", err)
	    return
    }

    // load from file
    c2, err := fastcache.LoadFromFile[string, int]("/tmp/cache.bin")
    if err != nil {
	    fmt.Println("load cache:", err)
	    return
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
                    │ fastcache_fork │               fastcache               │                 otter                  │
                    │     sec/op     │    sec/op     vs base                 │    sec/op     vs base                  │
  Set/1-4                20.37n ±  0%    44.07n ± 4%  +116.40% (p=0.000 n=10)   233.15n ± 1%  +1044.86% (p=0.000 n=10)
  Get/1-4                18.26n ±  0%    46.78n ± 0%  +156.16% (p=0.000 n=10)    33.83n ± 0%    +85.30% (p=0.000 n=10)
  SetGet/1-4             37.48n ±  0%    88.45n ± 0%  +135.98% (p=0.000 n=10)   278.35n ± 1%   +642.66% (p=0.000 n=10)
  GetParallel/1-4        43.91n ±  1%    48.23n ± 0%    +9.84% (p=0.000 n=10)    15.38n ± 1%    -64.98% (p=0.000 n=10)
  SetParallel/1-4        46.76n ±  2%    66.33n ± 1%   +41.88% (p=0.000 n=10)   251.80n ± 1%   +438.55% (p=0.000 n=10)
  Set/16-4               22.29n ±  0%    41.59n ± 2%   +86.65% (p=0.000 n=10)   237.90n ± 2%   +967.53% (p=0.000 n=10)
  Get/16-4               19.91n ±  0%    53.87n ± 2%  +170.57% (p=0.000 n=10)    35.33n ± 1%    +77.42% (p=0.000 n=10)
  SetGet/16-4            41.35n ±  0%    93.64n ± 1%  +126.44% (p=0.000 n=10)   279.05n ± 1%   +574.85% (p=0.000 n=10)
  GetParallel/16-4       46.31n ±  0%    46.90n ± 0%    +1.26% (p=0.000 n=10)    15.84n ± 0%    -65.80% (p=0.000 n=10)
  SetParallel/16-4       51.38n ±  3%    70.65n ± 1%   +37.50% (p=0.000 n=10)   255.00n ± 0%   +396.30% (p=0.000 n=10)
  Set/32-4               26.00n ±  0%    49.40n ± 0%   +89.98% (p=0.000 n=10)   238.75n ± 1%   +818.09% (p=0.000 n=10)
  Get/32-4               23.80n ±  0%    62.97n ± 0%  +164.61% (p=0.000 n=10)    36.14n ± 1%    +51.88% (p=0.000 n=10)
  SetGet/32-4            49.09n ±  0%   108.10n ± 0%  +120.21% (p=0.000 n=10)   281.80n ± 1%   +474.05% (p=0.000 n=10)
  GetParallel/32-4       34.55n ±  2%    41.28n ± 2%   +19.49% (p=0.000 n=10)    16.50n ± 0%    -52.24% (p=0.000 n=10)
  SetParallel/32-4       39.73n ± 10%    61.83n ± 1%   +55.59% (p=0.000 n=10)   232.40n ± 3%   +484.87% (p=0.000 n=10)
  Set/128-4              31.45n ±  0%    63.14n ± 2%  +100.75% (p=0.000 n=10)   242.05n ± 1%   +669.63% (p=0.000 n=10)
  Get/128-4              29.32n ±  0%    96.62n ± 1%  +229.46% (p=0.000 n=10)    38.24n ± 0%    +30.42% (p=0.000 n=10)
  SetGet/128-4           59.61n ±  0%   151.55n ± 0%  +154.21% (p=0.000 n=10)   295.50n ± 1%   +395.68% (p=0.000 n=10)
  GetParallel/128-4      16.50n ±  2%    65.49n ± 1%  +296.79% (p=0.000 n=10)    18.68n ± 0%    +13.18% (p=0.000 n=10)
  SetParallel/128-4      24.03n ± 11%    29.95n ± 8%   +24.64% (p=0.000 n=10)   240.90n ± 3%   +902.50% (p=0.000 n=10)
  Set/256-4              40.51n ±  0%    79.31n ± 1%   +95.82% (p=0.000 n=10)   247.30n ± 1%   +510.54% (p=0.000 n=10)
  Get/256-4              38.63n ±  1%   131.10n ± 1%  +239.37% (p=0.000 n=10)    51.40n ± 0%    +33.06% (p=0.000 n=10)
  SetGet/256-4           77.30n ±  0%   215.10n ± 0%  +178.27% (p=0.000 n=10)   300.95n ± 1%   +289.33% (p=0.000 n=10)
  GetParallel/256-4      20.40n ±  8%    78.72n ± 2%  +285.88% (p=0.000 n=10)    22.98n ± 0%    +12.67% (p=0.000 n=10)
  SetParallel/256-4      24.67n ±  9%    39.66n ± 5%   +60.77% (p=0.000 n=10)   248.85n ± 1%   +908.92% (p=0.000 n=10)
  Set/512-4              70.83n ±  0%   110.30n ± 0%   +55.74% (p=0.000 n=10)   258.20n ± 0%   +264.56% (p=0.000 n=10)
  Get/512-4              70.82n ±  1%   168.10n ± 1%  +137.36% (p=0.000 n=10)    58.24n ± 0%    -17.76% (p=0.000 n=10)
  SetGet/512-4           139.0n ±  0%    271.1n ± 0%   +95.04% (p=0.000 n=10)    319.6n ± 0%   +129.89% (p=0.000 n=10)
  GetParallel/512-4      30.91n ±  5%   100.40n ± 2%  +224.87% (p=0.000 n=10)    25.65n ± 0%    -17.00% (p=0.000 n=10)
  SetParallel/512-4      34.40n ±  6%    44.66n ± 3%   +29.81% (p=0.000 n=10)   250.15n ± 0%   +627.18% (p=0.000 n=10)
  Set/1024-4             97.08n ±  0%   175.70n ± 0%   +80.98% (p=0.000 n=10)   278.65n ± 1%   +187.02% (p=0.000 n=10)
  Get/1024-4             97.04n ±  0%   317.75n ± 3%  +227.44% (p=0.000 n=10)    73.27n ± 1%    -24.50% (p=0.000 n=10)
  SetGet/1024-4          191.2n ±  0%    437.9n ± 0%  +129.09% (p=0.000 n=10)    340.6n ± 0%    +78.18% (p=0.000 n=10)
  GetParallel/1024-4     42.58n ±  0%   161.60n ± 1%  +279.52% (p=0.000 n=10)    33.59n ± 0%    -21.11% (p=0.000 n=10)
  SetParallel/1024-4     44.45n ±  2%    74.70n ± 1%   +68.07% (p=0.000 n=10)   245.60n ± 1%   +452.53% (p=0.000 n=10)
  Set/2048-4             161.6n ±  0%    338.5n ± 0%  +109.47% (p=0.000 n=10)    330.7n ± 1%   +104.64% (p=0.000 n=10)
  Get/2048-4             160.3n ±  0%    669.9n ± 2%  +317.90% (p=0.000 n=10)    126.3n ± 2%    -21.21% (p=0.000 n=10)
  SetGet/2048-4          307.8n ±  0%    816.8n ± 1%  +165.41% (p=0.000 n=10)    415.6n ± 1%    +35.06% (p=0.000 n=10)
  GetParallel/2048-4     72.24n ±  2%   305.70n ± 1%  +323.17% (p=0.000 n=10)    52.02n ± 0%    -27.98% (p=0.000 n=10)
  SetParallel/2048-4     73.79n ±  1%   134.05n ± 1%   +81.66% (p=0.000 n=10)   227.75n ± 1%   +208.65% (p=0.000 n=10)
  Set/4096-4             310.0n ±  1%    649.1n ± 0%  +109.39% (p=0.000 n=10)    519.1n ± 1%    +67.44% (p=0.000 n=10)
  Get/4096-4             308.3n ±  1%   1068.5n ± 1%  +246.58% (p=0.000 n=10)    284.9n ± 0%     -7.59% (p=0.000 n=10)
  SetGet/4096-4          587.5n ±  0%   1629.5n ± 0%  +177.34% (p=0.000 n=10)    655.1n ± 1%    +11.51% (p=0.000 n=10)
  GetParallel/4096-4     131.1n ±  1%    526.8n ± 1%  +301.79% (p=0.000 n=10)    102.0n ± 3%    -22.20% (p=0.000 n=10)
  SetParallel/4096-4     133.7n ±  1%    254.6n ± 1%   +90.43% (p=0.000 n=10)    263.2n ± 3%    +96.86% (p=0.000 n=10)
  Set/8192-4             578.6n ±  1%   1204.0n ± 0%  +108.09% (p=0.000 n=10)    759.1n ± 2%    +31.20% (p=0.000 n=10)
  Get/8192-4             573.5n ±  1%   1907.5n ± 4%  +232.64% (p=0.000 n=10)    515.1n ± 1%    -10.17% (p=0.000 n=10)
  SetGet/8192-4         1072.0n ±  0%   3257.0n ± 1%  +203.82% (p=0.000 n=10)    965.4n ± 2%     -9.94% (p=0.000 n=10)
  GetParallel/8192-4     244.8n ±  0%    851.1n ± 1%  +247.74% (p=0.000 n=10)    188.2n ± 5%    -23.08% (p=0.000 n=10)
  SetParallel/8192-4     247.3n ±  0%    513.6n ± 3%  +107.72% (p=0.000 n=10)    375.2n ± 1%    +51.77% (p=0.000 n=10)
  geomean                69.81n          158.8n       +127.46%                   149.3n        +113.91%

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
  Set/32-4                0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/32-4                0.00 ± 0%       32.00 ± 0%  ? (p=0.000 n=10)        0.00 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/32-4             0.00 ± 0%       32.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/32-4        0.00 ± 0%       32.00 ± 0%  ? (p=0.000 n=10)        0.00 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/32-4        0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
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
  Get/512-4                0.0 ± 0%       490.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/512-4            0.00 ± 0%      512.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/512-4        0.0 ± 0%       490.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/512-4       0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/1024-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/1024-4               0.0 ± 0%       938.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/1024-4           0.00 ± 0%     1024.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/1024-4       0.0 ± 0%       938.0 ± 0%  ? (p=0.000 n=10)         0.0 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/1024-4      0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/2048-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/2048-4           0.000Ki ± 0%     1.749Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/2048-4           0.00 ± 0%     2048.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/2048-4   0.000Ki ± 0%     1.749Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/2048-4      0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/4096-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/4096-4           0.000Ki ± 0%     3.243Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/4096-4           0.00 ± 0%     4096.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/4096-4   0.000Ki ± 0%     3.239Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/4096-4      0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/8192-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/8192-4           0.000Ki ± 0%     6.478Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/8192-4           0.00 ± 0%     8192.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/8192-4   0.000Ki ± 0%     6.474Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
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
  Set/32-4               0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
  Get/32-4               0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/32-4            0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     1.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/32-4       0.000 ± 0%     1.000 ± 0%  ? (p=0.000 n=10)     0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/32-4       0.000 ± 0%     0.000 ± 0%  ~ (p=1.000 n=10) ¹   1.000 ± 0%  ? (p=0.000 n=10)
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

* **~2x+ faster** geomean sec/op vs original fastcache and otter.
* **Zero B/op** and **zero allocations** in measured ops; original fastcache still shows non-zero B/op across many read and mixed workloads, while otter incurs B/op and allocations on writes.
* Otter still wins several read-heavy micro-benchmarks, especially `GetParallel` and some larger-key `Get` cases; this fork leads decisively on writes and most mixed workloads.

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
