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
  cpu: AMD EPYC 9V74 80-Core Processor                
                    │ fastcache_fork │                fastcache                │                 otter                  │
                    │     sec/op     │    sec/op      vs base                  │    sec/op      vs base                 │
  Set/1-4                 23.97n ± 0%     39.18n ± 0%    +63.43% (p=0.000 n=10)   224.65n ±  1%  +837.21% (p=0.000 n=10)
  Get/1-4                 16.80n ± 1%     48.80n ± 0%   +190.42% (p=0.000 n=10)    35.44n ±  1%  +110.89% (p=0.000 n=10)
  SetGet/1-4              39.94n ± 0%     90.14n ± 1%   +125.72% (p=0.000 n=10)   263.25n ±  1%  +559.20% (p=0.000 n=10)
  GetParallel/1-4         52.75n ± 0%     66.99n ± 0%    +27.01% (p=0.000 n=10)    23.61n ±  1%   -55.25% (p=0.000 n=10)
  SetParallel/1-4         62.18n ± 3%     83.26n ± 0%    +33.91% (p=0.000 n=10)   258.55n ±  1%  +315.81% (p=0.000 n=10)
  Set/16-4                24.91n ± 0%     41.42n ± 0%    +66.30% (p=0.000 n=10)   227.60n ±  1%  +813.69% (p=0.000 n=10)
  Get/16-4                18.36n ± 0%     56.25n ± 0%   +206.37% (p=0.000 n=10)    35.97n ±  2%   +95.89% (p=0.000 n=10)
  SetGet/16-4             41.80n ± 0%     94.69n ± 0%   +126.49% (p=0.000 n=10)   265.25n ±  1%  +534.49% (p=0.000 n=10)
  GetParallel/16-4        52.90n ± 0%     60.69n ± 0%    +14.73% (p=0.000 n=10)    24.05n ±  0%   -54.53% (p=0.000 n=10)
  SetParallel/16-4        63.55n ± 3%     86.50n ± 0%    +36.11% (p=0.000 n=10)   258.65n ±  1%  +307.00% (p=0.000 n=10)
  Set/128-4               50.94n ± 0%     65.37n ± 1%    +28.31% (p=0.000 n=10)   232.85n ±  3%  +357.06% (p=0.000 n=10)
  Get/128-4               43.47n ± 1%     94.89n ± 1%   +118.29% (p=0.000 n=10)    39.23n ±  1%    -9.75% (p=0.000 n=10)
  SetGet/128-4            93.80n ± 0%    156.55n ± 3%    +66.91% (p=0.000 n=10)   274.95n ±  1%  +193.14% (p=0.000 n=10)
  GetParallel/128-4       33.23n ± 2%     74.30n ± 2%   +123.61% (p=0.000 n=10)    27.22n ±  1%   -18.06% (p=0.000 n=10)
  SetParallel/128-4       65.66n ± 0%    119.55n ± 0%    +82.07% (p=0.000 n=10)   228.50n ±  6%  +248.00% (p=0.000 n=10)
  Set/256-4               50.45n ± 1%     88.68n ± 0%    +75.76% (p=0.000 n=10)   236.00n ±  2%  +367.74% (p=0.000 n=10)
  Get/256-4               42.09n ± 0%    144.90n ± 1%   +244.26% (p=0.000 n=10)    46.32n ±  0%   +10.05% (p=0.000 n=10)
  SetGet/256-4            90.03n ± 0%    236.75n ± 0%   +162.95% (p=0.000 n=10)   275.90n ±  3%  +206.44% (p=0.000 n=10)
  GetParallel/256-4       28.63n ± 0%     94.71n ± 1%   +230.81% (p=0.000 n=10)    29.54n ±  1%    +3.18% (p=0.000 n=10)
  SetParallel/256-4       34.68n ± 1%     74.66n ± 2%   +115.27% (p=0.000 n=10)   243.05n ± 10%  +600.84% (p=0.000 n=10)
  Set/512-4               50.47n ± 2%    126.30n ± 1%   +150.22% (p=0.000 n=10)   239.50n ±  3%  +374.49% (p=0.000 n=10)
  Get/512-4               41.71n ± 0%    184.80n ± 2%   +343.06% (p=0.000 n=10)    59.31n ±  3%   +42.20% (p=0.000 n=10)
  SetGet/512-4            90.19n ± 0%    307.15n ± 0%   +240.56% (p=0.000 n=10)   288.30n ±  2%  +219.66% (p=0.000 n=10)
  GetParallel/512-4       28.71n ± 1%    114.40n ± 2%   +298.54% (p=0.000 n=10)    32.63n ±  3%   +13.67% (p=0.000 n=10)
  SetParallel/512-4       34.59n ± 0%    119.45n ± 1%   +245.33% (p=0.000 n=10)   247.30n ±  4%  +614.95% (p=0.000 n=10)
  Set/1024-4              51.90n ± 0%    190.65n ± 0%   +267.31% (p=0.000 n=10)   252.95n ±  2%  +387.33% (p=0.000 n=10)
  Get/1024-4              42.80n ± 1%    259.20n ± 1%   +505.54% (p=0.000 n=10)    83.94n ±  2%   +96.11% (p=0.000 n=10)
  SetGet/1024-4           91.69n ± 1%    460.00n ± 1%   +401.66% (p=0.000 n=10)   298.70n ±  3%  +225.75% (p=0.000 n=10)
  GetParallel/1024-4      28.70n ± 0%    151.75n ± 2%   +428.75% (p=0.000 n=10)    40.64n ±  2%   +41.59% (p=0.000 n=10)
  SetParallel/1024-4      34.51n ± 0%    109.35n ± 0%   +216.91% (p=0.000 n=10)   235.65n ±  3%  +582.94% (p=0.000 n=10)
  Set/2048-4              52.05n ± 0%    330.25n ± 0%   +534.43% (p=0.000 n=10)   266.00n ±  2%  +411.00% (p=0.000 n=10)
  Get/2048-4              42.88n ± 0%    490.25n ± 2%  +1043.31% (p=0.000 n=10)   127.60n ±  1%  +197.57% (p=0.000 n=10)
  SetGet/2048-4           91.59n ± 0%    830.00n ± 1%   +806.16% (p=0.000 n=10)   348.40n ±  3%  +280.37% (p=0.000 n=10)
  GetParallel/2048-4      28.64n ± 0%    272.75n ± 1%   +852.17% (p=0.000 n=10)    59.93n ±  1%  +109.22% (p=0.000 n=10)
  SetParallel/2048-4      34.56n ± 0%    163.75n ± 0%   +373.81% (p=0.000 n=10)   236.30n ±  2%  +583.74% (p=0.000 n=10)
  Set/4096-4              55.09n ± 2%    598.60n ± 1%   +986.59% (p=0.000 n=10)   307.25n ±  2%  +457.72% (p=0.000 n=10)
  Get/4096-4              45.72n ± 3%    955.90n ± 1%  +1991.00% (p=0.000 n=10)   203.55n ±  1%  +345.26% (p=0.000 n=10)
  SetGet/4096-4           96.06n ± 2%   1652.00n ± 1%  +1619.76% (p=0.000 n=10)   477.60n ±  2%  +397.19% (p=0.000 n=10)
  GetParallel/4096-4      29.04n ± 0%    521.90n ± 1%  +1697.18% (p=0.000 n=10)    99.00n ±  1%  +240.93% (p=0.000 n=10)
  SetParallel/4096-4      34.56n ± 0%    282.15n ± 0%   +716.52% (p=0.000 n=10)   238.05n ±  3%  +588.90% (p=0.000 n=10)
  Set/8192-4              120.8n ± 4%    1203.0n ± 0%   +895.86% (p=0.000 n=10)    579.9n ±  1%  +380.01% (p=0.000 n=10)
  Get/8192-4              115.5n ± 2%    2034.5n ± 6%  +1661.47% (p=0.000 n=10)    366.2n ±  1%  +217.10% (p=0.000 n=10)
  SetGet/8192-4           202.2n ± 2%    3296.5n ± 0%  +1529.91% (p=0.000 n=10)    819.0n ±  1%  +304.94% (p=0.000 n=10)
  GetParallel/8192-4      56.95n ± 0%   1023.50n ± 2%  +1697.19% (p=0.000 n=10)   174.15n ±  1%  +205.79% (p=0.000 n=10)
  SetParallel/8192-4      68.22n ± 2%    506.60n ± 0%   +642.60% (p=0.000 n=10)   292.75n ± 18%  +329.13% (p=0.000 n=10)
  geomean                 48.30n          198.8n        +311.56%                   151.3n        +213.20%

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
  SetGet/8192-4           0.00 ± 0%     8192.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 2%  ? (p=0.000 n=10)
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

* ~4.1x faster geomean `sec/op` vs original fastcache; ~3.1x faster vs otter.
* Zero allocations in measured ops; original fastcache still allocates across many read and mixed workloads, and otter allocates on writes.
* Otter still wins a few small-key read-heavy micro-benchmarks, mainly `GetParallel` up to 128-byte keys; this fork leads on the rest and pulls away hard on large keys.

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
