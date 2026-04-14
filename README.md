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
                    │ fastcache_fork │               fastcache                │                 otter                  │
                    │     sec/op     │    sec/op      vs base                 │    sec/op     vs base                  │
  Set/1-4                20.41n ±  1%    41.76n ±  1%  +104.53% (p=0.000 n=10)   218.55n ± 1%   +970.54% (p=0.000 n=10)
  Get/1-4                18.17n ±  0%    47.27n ±  1%  +160.13% (p=0.000 n=10)    33.71n ± 0%    +85.53% (p=0.000 n=10)
  SetGet/1-4             38.19n ±  0%    89.36n ±  0%  +134.03% (p=0.000 n=10)   276.85n ± 0%   +625.02% (p=0.000 n=10)
  GetParallel/1-4        44.93n ±  0%    47.84n ±  0%    +6.48% (p=0.000 n=10)    15.38n ± 1%    -65.77% (p=0.000 n=10)
  SetParallel/1-4        46.24n ±  4%    67.62n ±  1%   +46.24% (p=0.000 n=10)   250.15n ± 0%   +440.98% (p=0.000 n=10)
  Set/16-4               21.66n ±  0%    42.36n ±  0%   +95.57% (p=0.000 n=10)   222.60n ± 1%   +927.70% (p=0.000 n=10)
  Get/16-4               19.90n ±  0%    54.26n ±  0%  +172.64% (p=0.000 n=10)    35.36n ± 0%    +77.71% (p=0.000 n=10)
  SetGet/16-4            40.99n ±  0%    93.98n ±  2%  +129.29% (p=0.000 n=10)   279.05n ± 0%   +580.78% (p=0.000 n=10)
  GetParallel/16-4       45.97n ±  1%    45.92n ±  0%         ~ (p=0.671 n=10)    15.85n ± 0%    -65.52% (p=0.000 n=10)
  SetParallel/16-4       48.61n ±  0%    70.98n ±  0%   +46.04% (p=0.000 n=10)   251.50n ± 1%   +417.44% (p=0.000 n=10)
  Set/32-4               25.71n ±  0%    49.21n ±  2%   +91.37% (p=0.000 n=10)   223.60n ± 1%   +769.53% (p=0.000 n=10)
  Get/32-4               23.86n ±  0%    64.71n ±  0%  +171.26% (p=0.000 n=10)    36.00n ± 0%    +50.91% (p=0.000 n=10)
  SetGet/32-4            49.16n ±  1%   109.05n ±  2%  +121.83% (p=0.000 n=10)   281.75n ± 0%   +473.13% (p=0.000 n=10)
  GetParallel/32-4       35.82n ±  2%    42.42n ±  1%   +18.43% (p=0.000 n=10)    16.51n ± 0%    -53.91% (p=0.000 n=10)
  SetParallel/32-4       42.88n ±  7%    62.09n ±  1%   +44.81% (p=0.000 n=10)   231.50n ± 0%   +439.88% (p=0.000 n=10)
  Set/128-4              30.95n ±  0%    63.10n ±  1%  +103.88% (p=0.000 n=10)   231.60n ± 1%   +648.30% (p=0.000 n=10)
  Get/128-4              29.14n ±  0%   100.55n ±  0%  +245.00% (p=0.000 n=10)    38.30n ± 0%    +31.41% (p=0.000 n=10)
  SetGet/128-4           59.35n ±  0%   153.50n ±  1%  +158.64% (p=0.000 n=10)   290.75n ± 0%   +389.89% (p=0.000 n=10)
  GetParallel/128-4      17.46n ± 18%    66.92n ±  1%  +283.17% (p=0.000 n=10)    18.70n ± 0%          ~ (p=0.135 n=10)
  SetParallel/128-4      23.72n ± 12%    32.35n ± 10%   +36.40% (p=0.000 n=10)   245.75n ± 3%   +936.05% (p=0.000 n=10)
  Set/256-4              39.87n ±  0%    78.91n ±  1%   +97.91% (p=0.000 n=10)   239.10n ± 1%   +499.70% (p=0.000 n=10)
  Get/256-4              38.43n ±  0%   132.35n ±  1%  +244.35% (p=0.000 n=10)    42.80n ± 0%    +11.37% (p=0.000 n=10)
  SetGet/256-4           76.59n ±  0%   214.25n ±  1%  +179.74% (p=0.000 n=10)   306.05n ± 1%   +299.60% (p=0.000 n=10)
  GetParallel/256-4      20.02n ±  1%    80.87n ±  2%  +304.05% (p=0.000 n=10)    20.77n ± 0%     +3.80% (p=0.000 n=10)
  SetParallel/256-4      22.01n ± 12%    39.18n ±  3%   +78.01% (p=0.000 n=10)   245.55n ± 1%  +1015.63% (p=0.000 n=10)
  Set/512-4              71.24n ±  0%   109.00n ±  0%   +52.99% (p=0.000 n=10)   254.90n ± 1%   +257.78% (p=0.000 n=10)
  Get/512-4              70.94n ±  0%   174.40n ±  1%  +145.82% (p=0.000 n=10)    54.04n ± 0%    -23.82% (p=0.000 n=10)
  SetGet/512-4           139.2n ±  0%    274.3n ±  1%   +97.02% (p=0.000 n=10)    322.5n ± 0%   +131.60% (p=0.000 n=10)
  GetParallel/512-4      30.32n ±  9%   102.70n ±  2%  +238.66% (p=0.000 n=10)    24.55n ± 1%    -19.03% (p=0.000 n=10)
  SetParallel/512-4      33.37n ±  8%    44.04n ±  1%   +31.99% (p=0.000 n=10)   246.25n ± 1%   +638.05% (p=0.000 n=10)
  Set/1024-4             99.62n ±  0%   177.90n ±  1%   +78.57% (p=0.000 n=10)   274.25n ± 1%   +175.28% (p=0.000 n=10)
  Get/1024-4             98.88n ±  0%   349.90n ±  3%  +253.88% (p=0.000 n=10)    71.42n ± 0%    -27.77% (p=0.000 n=10)
  SetGet/1024-4          195.8n ±  1%    443.7n ±  1%  +126.69% (p=0.000 n=10)    336.8n ± 1%    +72.03% (p=0.000 n=10)
  GetParallel/1024-4     42.98n ±  5%   168.35n ±  1%  +291.65% (p=0.000 n=10)    32.46n ± 0%    -24.47% (p=0.000 n=10)
  SetParallel/1024-4     44.12n ±  1%    75.04n ±  3%   +70.09% (p=0.000 n=10)   244.65n ± 0%   +454.57% (p=0.000 n=10)
  Set/2048-4             169.6n ±  1%    348.4n ±  1%  +105.36% (p=0.000 n=10)    334.3n ± 1%    +97.05% (p=0.000 n=10)
  Get/2048-4             168.1n ±  0%    702.3n ±  3%  +317.91% (p=0.000 n=10)    124.8n ± 2%    -25.77% (p=0.000 n=10)
  SetGet/2048-4          322.6n ±  0%    823.2n ±  2%  +155.22% (p=0.000 n=10)    417.4n ± 1%    +29.42% (p=0.000 n=10)
  GetParallel/2048-4     73.00n ±  1%   326.90n ±  3%  +347.78% (p=0.000 n=10)    50.83n ± 0%    -30.38% (p=0.000 n=10)
  SetParallel/2048-4     74.21n ±  1%   136.00n ±  1%   +83.26% (p=0.000 n=10)   230.45n ± 1%   +210.54% (p=0.000 n=10)
  Set/4096-4             324.9n ±  1%    660.8n ±  2%  +103.43% (p=0.000 n=10)    546.1n ± 2%    +68.09% (p=0.000 n=10)
  Get/4096-4             326.4n ±  1%   1120.0n ±  2%  +243.08% (p=0.000 n=10)    291.8n ± 1%    -10.60% (p=0.000 n=10)
  SetGet/4096-4          620.5n ±  0%   1661.5n ±  2%  +167.77% (p=0.000 n=10)    685.7n ± 1%    +10.52% (p=0.000 n=10)
  GetParallel/4096-4     132.7n ±  3%    549.5n ±  2%  +314.09% (p=0.000 n=10)    103.8n ± 2%    -21.74% (p=0.000 n=10)
  SetParallel/4096-4     135.3n ±  1%    274.5n ±  1%  +102.88% (p=0.000 n=10)    271.1n ± 3%   +100.33% (p=0.000 n=10)
  Set/8192-4             610.4n ±  1%   1234.5n ±  2%  +102.24% (p=0.000 n=10)    826.2n ± 1%    +35.35% (p=0.000 n=10)
  Get/8192-4             597.7n ±  1%   1990.5n ±  5%  +233.03% (p=0.000 n=10)    535.2n ± 2%    -10.46% (p=0.000 n=10)
  SetGet/8192-4         1138.5n ±  1%   3326.0n ±  1%  +192.14% (p=0.000 n=10)    972.6n ± 2%    -14.58% (p=0.000 n=10)
  GetParallel/8192-4     251.2n ±  1%    886.9n ±  1%  +253.09% (p=0.000 n=10)    193.2n ± 3%    -23.11% (p=0.000 n=10)
  SetParallel/8192-4     250.6n ±  2%    537.0n ±  4%  +114.31% (p=0.000 n=10)    381.8n ± 1%    +52.35% (p=0.000 n=10)
  geomean                70.45n          161.9n        +129.86%                   147.9n        +109.95%

                    │ fastcache_fork │            fastcache             │              otter               │
                    │      B/op      │     B/op      vs base            │     B/op      vs base            │
  Set/1-4                 0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/1-4                0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)       0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/1-4             0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)      64.000 ± 0%  ? (p=0.000 n=10)
  GetParallel/1-4        0.000 ± 0%       8.000 ± 0%  ? (p=0.000 n=10)       0.000 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/1-4         0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     65.00 ± 2%  ? (p=0.000 n=10)
  Set/16-4                0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/16-4                0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)        0.00 ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/16-4             0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/16-4        0.00 ± 0%       16.00 ± 0%  ? (p=0.000 n=10)        0.00 ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/16-4        0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.50 ± 1%  ? (p=0.000 n=10)
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
  GetParallel/4096-4   0.000Ki ± 0%     3.241Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetParallel/4096-4      0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Set/8192-4              0.00 ± 0%        0.00 ± 0%  ~ (p=1.000 n=10) ¹     64.00 ± 0%  ? (p=0.000 n=10)
  Get/8192-4           0.000Ki ± 0%     6.462Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
  SetGet/8192-4           0.00 ± 0%     8192.00 ± 0%  ? (p=0.000 n=10)       64.00 ± 0%  ? (p=0.000 n=10)
  GetParallel/8192-4   0.000Ki ± 0%     6.504Ki ± 0%  ? (p=0.000 n=10)     0.000Ki ± 0%  ~ (p=1.000 n=10) ¹
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

* ~2.3x faster geomean `sec/op` vs original fastcache; ~2.1x faster vs otter.
* Zero allocations in measured ops; original fastcache still allocates across many read and mixed workloads, and otter allocates on writes.
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
