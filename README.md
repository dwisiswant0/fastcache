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
* **Atomic operations**: `GetOrSet`, `GetAndDelete`, `SetIfAbsent` check and update a key in one step, with no locking in your code.
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

* **512 shards**: Writers lock only the shard of their key, reducing contention on multi-core CPUs.
* **Open-addressing hash table per shard**: Entries are stored by the rapidhash value of their key, for O(1) lookups.
* **`Get` without locking**: Readers probe the table with atomic loads, so reading one key scales with the number of CPUs. A miss that overlaps a delete in the same shard probes again under the shard's lock. Hit and miss counts are striped per CPU.
* **Cache-wide FIFO queue**: One queue tracks insertion order across all shards, so the oldest entry in the cache is evicted first. Adding a new key also locks this queue.

## Differences from Original

| Aspect | Original fastcache | This fork |
|--------|-------------------|-----------|
| **API** | `[]byte` keys/values | Generic `[K, V]` |
| **Capacity** | Bytes-based | Entry-count based |
| **Storage** | Ring buffer of bytes | Open-addressing hash table per shard |
| **Eviction** | FIFO (by byte position) | FIFO (by insertion order) |
| **Allocations** | 1 alloc/Get | Zero |

## Benchmarks

Compared against the original [VictoriaMetrics/fastcache](https://github.com/VictoriaMetrics/fastcache) and [maypok86/otter](https://github.com/maypok86/otter):

<details open>
  <summary><code>benchstat</code></summary>

  ```
  goos: darwin
  goarch: arm64
  pkg: benchmarks
  cpu: Apple M5 Pro
                     │ fastcache_fork │                fastcache                │                  otter                  │
                     │     sec/op     │    sec/op      vs base                  │    sec/op      vs base                  │
  Set/1-4               29.12n ±   4%    23.98n ±  1%    -17.65% (p=0.000 n=10)   287.65n ±  2%   +887.81% (p=0.000 n=10)
  Get/1-4               12.27n ±   0%    25.76n ±  1%   +109.86% (p=0.000 n=10)    20.71n ±  3%    +68.72% (p=0.000 n=10)
  SetGet/1-4            42.56n ±   7%    59.37n ±  2%    +39.49% (p=0.000 n=10)   324.75n ±  2%   +663.04% (p=0.000 n=10)
  GetParallel/1-4       6.878n ±  33%   71.425n ±  1%   +938.46% (p=0.000 n=10)    5.880n ±  3%    -14.52% (p=0.023 n=10)
  SetParallel/1-4       137.9n ±   3%    218.2n ±  1%    +58.25% (p=0.000 n=10)    376.9n ±  2%   +173.38% (p=0.000 n=10)
  Set/16-4              30.03n ±  82%    25.92n ±  4%    -13.69% (p=0.001 n=10)   279.60n ±  3%   +831.22% (p=0.000 n=10)
  Get/16-4              12.44n ±   0%    30.41n ±  0%   +144.45% (p=0.000 n=10)    21.21n ±  1%    +70.54% (p=0.000 n=10)
  SetGet/16-4           42.04n ±   4%    60.61n ±  1%    +44.19% (p=0.000 n=10)   330.10n ±  1%   +685.30% (p=0.000 n=10)
  GetParallel/16-4      5.034n ±  41%   79.715n ± 21%  +1483.69% (p=0.000 n=10)    5.476n ±  5%          ~ (p=0.247 n=10)
  SetParallel/16-4      151.9n ±   1%    218.9n ± 21%    +44.06% (p=0.002 n=10)    383.2n ±  1%   +152.19% (p=0.000 n=10)
  Set/32-4              31.86n ±   4%    32.16n ±  1%          ~ (p=0.066 n=10)   285.15n ±  1%   +794.87% (p=0.000 n=10)
  Get/32-4              14.58n ±   2%    36.58n ±  1%   +150.89% (p=0.000 n=10)    22.72n ± 26%    +55.83% (p=0.000 n=10)
  SetGet/32-4           45.53n ±   5%    70.96n ±  0%    +55.87% (p=0.000 n=10)   332.55n ±  1%   +630.48% (p=0.000 n=10)
  GetParallel/32-4      7.906n ±   1%   49.145n ±  0%   +521.62% (p=0.000 n=10)    5.745n ±  6%    -27.33% (p=0.001 n=10)
  SetParallel/32-4      107.2n ±   3%    138.2n ±  8%    +28.90% (p=0.000 n=10)    267.6n ±  3%   +149.51% (p=0.000 n=10)
  Set/128-4             34.64n ±   6%    43.18n ±  1%    +24.65% (p=0.000 n=10)   288.45n ±  1%   +732.71% (p=0.000 n=10)
  Get/128-4             18.39n ±   2%    57.26n ±  2%   +211.42% (p=0.000 n=10)    27.02n ± 14%    +46.97% (p=0.000 n=10)
  SetGet/128-4          52.31n ±   6%    99.08n ±  1%    +89.40% (p=0.002 n=10)   335.00n ±  0%   +540.41% (p=0.002 n=10)
  GetParallel/128-4     9.747n ± 228%   46.170n ±  2%   +373.68% (p=0.001 n=10)    6.673n ±  4%    -31.53% (p=0.000 n=10)
  SetParallel/128-4     29.92n ±  18%    45.63n ± 35%    +52.51% (p=0.003 n=10)   251.75n ±  7%   +741.41% (p=0.000 n=10)
  Set/256-4             39.61n ±   5%    58.98n ±  1%    +48.91% (p=0.000 n=10)   284.90n ±  0%   +619.35% (p=0.000 n=10)
  Get/256-4             24.49n ±   7%    79.27n ±  2%   +223.66% (p=0.000 n=10)    30.39n ±  1%    +24.09% (p=0.000 n=10)
  SetGet/256-4          64.32n ±   3%   142.80n ±  2%   +122.01% (p=0.000 n=10)   335.15n ±  2%   +421.07% (p=0.000 n=10)
  GetParallel/256-4     9.661n ±   5%   56.825n ±  2%   +488.19% (p=0.000 n=10)    7.787n ±  6%    -19.39% (p=0.000 n=10)
  SetParallel/256-4     19.07n ±  31%    28.66n ± 20%    +50.29% (p=0.000 n=10)   254.65n ±  4%  +1235.34% (p=0.000 n=10)
  Set/512-4             56.61n ± 132%    88.84n ±  0%    +56.93% (p=0.023 n=10)   282.95n ±  1%   +399.82% (p=0.000 n=10)
  Get/512-4             37.16n ±   3%   119.95n ±  1%   +222.84% (p=0.000 n=10)    40.49n ±  2%     +8.96% (p=0.000 n=10)
  SetGet/512-4          91.97n ±   4%   207.90n ±  1%   +126.05% (p=0.000 n=10)   340.60n ±  2%   +270.34% (p=0.000 n=10)
  GetParallel/512-4    13.540n ±  10%   87.230n ±  2%   +544.24% (p=0.000 n=10)    9.807n ±  2%    -27.57% (p=0.000 n=10)
  SetParallel/512-4     20.90n ±  20%    22.66n ± 21%     +8.40% (p=0.001 n=10)   261.60n ±  3%  +1151.67% (p=0.000 n=10)
  Set/1024-4            79.09n ±   2%   139.00n ±  0%    +75.75% (p=0.000 n=10)   297.25n ±  1%   +275.84% (p=0.000 n=10)
  Get/1024-4            59.91n ±   1%   180.55n ±  1%   +201.39% (p=0.000 n=10)    58.01n ±  2%     -3.16% (p=0.000 n=10)
  SetGet/1024-4         140.5n ±   1%    322.2n ±  1%   +129.44% (p=0.000 n=10)    355.1n ±  1%   +152.79% (p=0.000 n=10)
  GetParallel/1024-4    18.04n ±  11%   117.00n ±  3%   +548.38% (p=0.000 n=10)    14.56n ±  1%    -19.31% (p=0.000 n=10)
  SetParallel/1024-4    27.99n ± 360%    38.41n ±  5%          ~ (p=0.481 n=10)   259.65n ±  4%   +827.65% (p=0.001 n=10)
  Set/2048-4            128.3n ±   1%    235.4n ±  1%    +83.52% (p=0.000 n=10)    310.6n ±  1%   +142.09% (p=0.000 n=10)
  Get/2048-4            107.2n ±  12%    336.8n ± 44%   +213.99% (p=0.000 n=10)    103.6n ±  3%     -3.36% (p=0.001 n=10)
  SetGet/2048-4         236.4n ±   1%    601.0n ±  6%   +154.23% (p=0.000 n=10)    375.0n ±  1%    +58.63% (p=0.000 n=10)
  GetParallel/2048-4    36.69n ±   9%   199.80n ±  3%   +444.56% (p=0.000 n=10)    28.80n ±  3%    -21.49% (p=0.001 n=10)
  SetParallel/2048-4    41.25n ±  15%    77.94n ±  8%    +88.96% (p=0.000 n=10)   184.55n ±  6%   +347.39% (p=0.000 n=10)
  Set/4096-4            223.0n ±   1%    452.2n ±  0%   +102.80% (p=0.000 n=10)    377.1n ±  0%    +69.08% (p=0.000 n=10)
  Get/4096-4            204.3n ±   1%    602.9n ±  2%   +195.11% (p=0.000 n=10)    172.3n ±  0%    -15.66% (p=0.000 n=10)
  SetGet/4096-4         415.0n ±   0%   1137.5n ±  1%   +174.06% (p=0.000 n=10)    479.2n ±  1%    +15.47% (p=0.000 n=10)
  GetParallel/4096-4    65.54n ±  21%   418.85n ± 10%   +539.12% (p=0.000 n=10)    57.16n ±  3%    -12.78% (p=0.001 n=10)
  SetParallel/4096-4    87.06n ±  12%   173.80n ±  7%    +99.64% (p=0.000 n=10)   204.50n ±  1%   +134.91% (p=0.000 n=10)
  Set/8192-4            404.5n ±   2%    876.0n ±  0%   +116.52% (p=0.000 n=10)    527.7n ±  1%    +30.43% (p=0.000 n=10)
  Get/8192-4            392.2n ±   4%   1396.0n ±  4%   +255.94% (p=0.000 n=10)    300.4n ±  1%    -23.39% (p=0.000 n=10)
  SetGet/8192-4         779.5n ±   0%   2375.0n ±  7%   +204.68% (p=0.000 n=10)    680.5n ±  1%    -12.69% (p=0.002 n=10)
  GetParallel/8192-4   123.65n ±  49%   644.10n ±  6%   +420.91% (p=0.000 n=10)    98.73n ±  3%    -20.15% (p=0.000 n=10)
  SetParallel/8192-4    132.5n ±  20%    298.0n ±  7%   +124.87% (p=0.000 n=10)    237.7n ±  1%    +79.36% (p=0.000 n=10)
  geomean               49.74n           122.2n         +145.65%                   116.4n         +134.08%

                     │ fastcache_fork │                 fastcache                 │                 otter                 │
                     │      B/op      │     B/op      vs base                     │     B/op      vs base                 │
  Set/1-4                64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 0%  +1.56% (p=0.000 n=10)
  Get/1-4                0.000 ± 0%       8.000 ± 0%           ? (p=0.000 n=10)       0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/1-4            64.000 ± 0%       8.000 ± 0%     -87.50% (p=0.000 n=10)      66.000 ± 2%  +3.12% (p=0.000 n=10)
  GetParallel/1-4        0.000 ± 0%       8.000 ± 0%           ? (p=0.000 n=10)       0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/1-4        64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       69.00 ± 1%  +7.81% (p=0.000 n=10)
  Set/16-4               64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 0%  +1.56% (p=0.000 n=10)
  Get/16-4                0.00 ± 0%       16.00 ± 0%           ? (p=0.000 n=10)        0.00 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/16-4            64.00 ± 0%       16.00 ± 0%     -75.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  GetParallel/16-4        0.00 ± 0%       16.00 ± 0%           ? (p=0.000 n=10)        0.00 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/16-4       64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       69.00 ± 0%  +7.81% (p=0.000 n=10)
  Set/32-4               64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 0%  +1.56% (p=0.000 n=10)
  Get/32-4                0.00 ± 0%       32.00 ± 0%           ? (p=0.000 n=10)        0.00 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/32-4            64.00 ± 0%       32.00 ± 0%     -50.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  GetParallel/32-4        0.00 ± 0%       32.00 ± 0%           ? (p=0.000 n=10)        0.00 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/32-4       64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 0%  +1.56% (p=0.000 n=10)
  Set/128-4              64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 0%  +1.56% (p=0.000 n=10)
  Get/128-4                0.0 ± 0%       128.0 ± 0%           ? (p=0.000 n=10)         0.0 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/128-4           64.00 ± 0%      128.00 ± 0%    +100.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  GetParallel/128-4        0.0 ± 0%       128.0 ± 0%           ? (p=0.000 n=10)         0.0 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/128-4      64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  Set/256-4              64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 0%  +1.56% (p=0.000 n=10)
  Get/256-4                0.0 ± 0%       234.0 ± 0%           ? (p=0.000 n=10)         0.0 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/256-4           64.00 ± 0%      256.00 ± 0%    +300.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  GetParallel/256-4        0.0 ± 0%       234.0 ± 0%           ? (p=0.000 n=10)         0.0 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/256-4      64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  Set/512-4              64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 0%  +1.56% (p=0.000 n=10)
  Get/512-4                0.0 ± 0%       490.0 ± 0%           ? (p=0.000 n=10)         0.0 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/512-4           64.00 ± 0%      512.00 ± 0%    +700.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  GetParallel/512-4        0.0 ± 0%       490.0 ± 0%           ? (p=0.000 n=10)         0.0 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/512-4      64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  Set/1024-4             64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.50 ± 1%  +2.34% (p=0.000 n=10)
  Get/1024-4               0.0 ± 0%       938.0 ± 0%           ? (p=0.000 n=10)         0.0 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/1024-4          64.00 ± 0%     1024.00 ± 0%   +1500.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  GetParallel/1024-4       0.0 ± 0%       938.0 ± 0%           ? (p=0.000 n=10)         0.0 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/1024-4     64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  Set/2048-4             64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  Get/2048-4           0.000Ki ± 0%     1.749Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/2048-4          64.00 ± 0%     2048.00 ± 0%   +3100.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  GetParallel/2048-4   0.000Ki ± 0%     1.749Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/2048-4     64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  Set/4096-4             64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       66.00 ± 0%  +3.12% (p=0.000 n=10)
  Get/4096-4           0.000Ki ± 0%     3.247Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/4096-4          64.00 ± 0%     4096.00 ± 0%   +6300.00% (p=0.000 n=10)       67.00 ± 0%  +4.69% (p=0.000 n=10)
  GetParallel/4096-4   0.000Ki ± 0%     3.243Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/4096-4     64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  Set/8192-4             64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       67.00 ± 0%  +4.69% (p=0.000 n=10)
  Get/8192-4           0.000Ki ± 0%     6.479Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/8192-4          64.00 ± 0%     8192.00 ± 0%  +12700.00% (p=0.000 n=10)       68.00 ± 1%  +6.25% (p=0.000 n=10)
  GetParallel/8192-4   0.000Ki ± 0%     6.454Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/8192-4     64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  geomean                           ²                 ?                         ²                 +1.50%                ²
  ¹ all samples are equal
  ² summaries must be >0 to compute geomean

                     │ fastcache_fork │               fastcache               │                otter                │
                     │   allocs/op    │ allocs/op   vs base                   │ allocs/op   vs base                 │
  Set/1-4                1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/1-4                0.000 ± 0%     1.000 ± 0%         ? (p=0.000 n=10)     0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/1-4             1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/1-4        0.000 ± 0%     1.000 ± 0%         ? (p=0.000 n=10)     0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/1-4        1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/16-4               1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/16-4               0.000 ± 0%     1.000 ± 0%         ? (p=0.000 n=10)     0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/16-4            1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/16-4       0.000 ± 0%     1.000 ± 0%         ? (p=0.000 n=10)     0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/16-4       1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/32-4               1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/32-4               0.000 ± 0%     1.000 ± 0%         ? (p=0.000 n=10)     0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/32-4            1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/32-4       0.000 ± 0%     1.000 ± 0%         ? (p=0.000 n=10)     0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/32-4       1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/128-4              1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/128-4              0.000 ± 0%     1.000 ± 0%         ? (p=0.000 n=10)     0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/128-4           1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/128-4      0.000 ± 0%     1.000 ± 0%         ? (p=0.000 n=10)     0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/128-4      1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/256-4              1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/256-4              0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/256-4           1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/256-4      0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/256-4      1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/512-4              1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/512-4              0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/512-4           1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/512-4      0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/512-4      1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/1024-4             1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/1024-4             0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/1024-4          1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/1024-4     0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/1024-4     1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/2048-4             1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/2048-4             0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/2048-4          1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/2048-4     0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/2048-4     1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/4096-4             1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/4096-4             0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/4096-4          1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/4096-4     0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/4096-4     1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Set/8192-4             1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  Get/8192-4             0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/8192-4          1.000 ± 0%     1.000 ± 0%         ~ (p=1.000 n=10) ¹   1.000 ± 0%       ~ (p=1.000 n=10) ¹
  GetParallel/8192-4     0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹   0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/8192-4     1.000 ± 0%     0.000 ± 0%  -100.00% (p=0.000 n=10)     1.000 ± 0%       ~ (p=1.000 n=10) ¹
  geomean                           ²               ?                       ²               +0.00%                ²
  ¹ all samples are equal
  ² summaries must be >0 to compute geomean
  ```
</details>

Highlights:

* **2.3x to 2.5x faster** geomean sec/op than otter and the original fastcache (49.7 ns vs 116.4 ns and 122.2 ns).
* **Zero allocations on `Get`**. A `Set` that replaces a value allocates the new entry (64 B/op here), about what otter allocates on every write; the original fastcache allocates on reads instead.
* **`GetParallel`** is 5x to 16x faster than the original fastcache. Otter still takes 13% to 32% less time in 9 of 10 `GetParallel` sizes, and 3% to 23% less in `Get` from 1024 keys up.
* **Writes**: this fork wins every `Set` and `SetParallel` case against otter. Against the original fastcache it wins every `SetGet` case and `Set` from 128 keys up; the original is faster for `Set` on 1 and 16 keys.

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
