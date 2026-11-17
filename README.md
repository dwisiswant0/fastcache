# fastcache

[![Go Reference](https://pkg.go.dev/badge/go.dw1.io/fastcache.svg)](https://pkg.go.dev/go.dw1.io/fastcache)

A fast, generic, thread-safe cache for Go.

> [!NOTE]
> This is a fork of [VictoriaMetrics/fastcache](https://github.com/VictoriaMetrics/fastcache) with a redesigned API using Go generics.

## Features

* **Generic**: Type-safe API.
* **No allocations on `Get` with string keys**: Other key types can allocate once per `Get` while hashing, and every `Set` allocates the entry it stores.
* **Thread-safe**: Concurrent goroutines may read and write into a single cache instance.
* **FIFO eviction**: Oldest entries are evicted first when the cache is full.
* **Weighted capacity**: `NewWeighted` also bounds the total weight of the entries, such as the bytes of their values.
* **Iterators**: Go 1.23+ range-over-func support with `All()`, `Keys()`, `Values()`, and their eviction-order twins `AllFIFO()`, `KeysFIFO()`, `ValuesFIFO()`.
* **Atomic operations**: `GetOrSet`, `GetAndDelete`, `SetIfAbsent` check and update a key in one step, with no locking in your code.
* **Persistence**: Cache can be saved to file and loaded from file, whole or through functions that select and convert entries (`SaveToFunc`, `LoadFromFunc`).
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
* **Cache-wide FIFO queue**: One queue tracks insertion order across all shards, so the oldest entry in the cache is evicted first. Adding a new key also locks this queue, as does making a value heavier in a cache made by `NewWeighted`.

## Differences from Original

| Aspect | Original fastcache | This fork |
|--------|-------------------|-----------|
| **API** | `[]byte` keys/values | Generic `[K, V]` |
| **Capacity** | Bytes-based | Entry count, plus total weight with `NewWeighted` |
| **Storage** | Ring buffer of bytes | Open-addressing hash table per shard |
| **Eviction** | FIFO (by byte position) | FIFO (by insertion order) |
| **Allocations** | Get with nil dst: 1 per hit. Set: 0 | Get with string keys: 0. Set: 1 or more |

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
  Set/1-4                29.65n ± 13%    24.25n ±  9%    -18.20% (p=0.000 n=10)   291.25n ±  1%   +882.29% (p=0.000 n=10)
  Get/1-4                12.21n ±  2%    25.73n ±  2%   +110.73% (p=0.000 n=10)    20.49n ±  0%    +67.81% (p=0.000 n=10)
  SetGet/1-4             42.88n ±  5%    56.49n ±  6%    +31.72% (p=0.000 n=10)   336.80n ±  1%   +685.36% (p=0.000 n=10)
  GetParallel/1-4        6.983n ±  1%   70.680n ±  1%   +912.17% (p=0.000 n=10)    5.274n ±  1%    -24.48% (p=0.000 n=10)
  SetParallel/1-4        145.4n ±  2%    228.3n ± 17%    +57.05% (p=0.000 n=10)    378.5n ±  1%   +160.32% (p=0.000 n=10)
  Set/16-4               30.25n ±  6%    26.11n ±  2%    -13.66% (p=0.000 n=10)   289.05n ±  1%   +855.70% (p=0.000 n=10)
  Get/16-4               12.46n ±  1%    30.17n ±  1%   +142.13% (p=0.000 n=10)    21.14n ±  5%    +69.66% (p=0.000 n=10)
  SetGet/16-4            42.83n ±  5%    58.95n ±  4%    +37.65% (p=0.000 n=10)   335.70n ±  2%   +683.80% (p=0.000 n=10)
  GetParallel/16-4       7.048n ± 34%   78.370n ±  0%  +1011.87% (p=0.000 n=10)    5.202n ±  0%    -26.19% (p=0.022 n=10)
  SetParallel/16-4       158.8n ±  4%    258.1n ± 16%    +62.58% (p=0.000 n=10)    383.3n ±  1%   +141.48% (p=0.000 n=10)
  Set/32-4               33.32n ± 12%    31.92n ±  2%          ~ (p=0.143 n=10)   289.80n ±  1%   +769.88% (p=0.000 n=10)
  Get/32-4               14.59n ±  0%    36.17n ±  0%   +147.99% (p=0.000 n=10)    21.67n ±  1%    +48.54% (p=0.000 n=10)
  SetGet/32-4            46.46n ±  4%    70.96n ±  2%    +52.72% (p=0.000 n=10)   336.15n ±  1%   +623.45% (p=0.000 n=10)
  GetParallel/32-4       7.839n ± 34%   48.940n ±  0%   +524.35% (p=0.000 n=10)    5.414n ±  7%    -30.93% (p=0.023 n=10)
  SetParallel/32-4       109.8n ±  2%    146.4n ±  4%    +33.23% (p=0.000 n=10)    267.2n ±  1%   +143.29% (p=0.000 n=10)
  Set/128-4              35.80n ±  7%    42.03n ±  1%    +17.37% (p=0.000 n=10)   290.60n ±  1%   +711.62% (p=0.000 n=10)
  Get/128-4              18.18n ±  1%    55.64n ±  1%   +206.11% (p=0.000 n=10)    26.47n ±  1%    +45.64% (p=0.000 n=10)
  SetGet/128-4           52.43n ±  6%    96.25n ±  1%    +83.57% (p=0.000 n=10)   339.35n ±  1%   +547.24% (p=0.000 n=10)
  GetParallel/128-4      6.643n ± 33%   44.620n ±  0%   +571.68% (p=0.000 n=10)    6.649n ± 12%          ~ (p=0.684 n=10)
  SetParallel/128-4      28.45n ± 11%    27.21n ± 73%          ~ (p=0.971 n=10)   239.80n ± 17%   +743.03% (p=0.000 n=10)
  Set/256-4              40.55n ±  1%    59.01n ±  1%    +45.54% (p=0.000 n=10)   290.10n ±  1%   +615.50% (p=0.000 n=10)
  Get/256-4              24.33n ±  1%    78.83n ±  1%   +224.00% (p=0.000 n=10)    30.38n ±  4%    +24.87% (p=0.000 n=10)
  SetGet/256-4           66.20n ±  5%   139.25n ±  0%   +110.35% (p=0.000 n=10)   341.05n ±  1%   +415.18% (p=0.000 n=10)
  GetParallel/256-4      9.428n ±  1%   58.035n ±  1%   +515.53% (p=0.000 n=10)    7.609n ±  1%    -19.29% (p=0.002 n=10)
  SetParallel/256-4      14.93n ± 34%    27.95n ± 26%    +87.24% (p=0.000 n=10)   254.90n ±  3%  +1607.87% (p=0.000 n=10)
  Set/512-4              53.08n ±  5%    88.25n ±  1%    +66.26% (p=0.000 n=10)   288.75n ±  1%   +443.99% (p=0.000 n=10)
  Get/512-4              36.19n ±  2%   118.30n ±  1%   +226.93% (p=0.000 n=10)    40.48n ±  2%    +11.86% (p=0.000 n=10)
  SetGet/512-4           90.88n ±  1%   202.55n ±  0%   +122.88% (p=0.000 n=10)   342.10n ±  2%   +276.43% (p=0.000 n=10)
  GetParallel/512-4     12.745n ±  1%   84.205n ±  1%   +560.69% (p=0.000 n=10)    9.821n ±  1%    -22.94% (p=0.000 n=10)
  SetParallel/512-4      21.50n ± 23%    22.46n ±  1%     +4.44% (p=0.000 n=10)   265.55n ±  5%  +1134.83% (p=0.000 n=10)
  Set/1024-4             78.39n ±  5%   138.30n ±  1%    +76.43% (p=0.000 n=10)   295.35n ±  1%   +276.77% (p=0.000 n=10)
  Get/1024-4             58.87n ±  1%   179.30n ±  1%   +204.57% (p=0.000 n=10)    58.20n ±  0%     -1.14% (p=0.001 n=10)
  SetGet/1024-4          137.6n ±  1%    319.7n ±  3%   +132.34% (p=0.000 n=10)    353.1n ±  1%   +156.61% (p=0.000 n=10)
  GetParallel/1024-4     17.38n ±  7%   117.10n ±  1%   +573.96% (p=0.000 n=10)    14.56n ±  3%    -16.20% (p=0.000 n=10)
  SetParallel/1024-4     24.01n ± 11%    36.70n ±  3%    +52.85% (p=0.000 n=10)   255.90n ±  4%   +965.81% (p=0.000 n=10)
  Set/2048-4             124.1n ±  2%    231.8n ±  1%    +86.78% (p=0.000 n=10)    314.8n ±  1%   +153.63% (p=0.000 n=10)
  Get/2048-4             106.2n ±  0%    328.6n ±  6%   +209.37% (p=0.000 n=10)    103.4n ±  3%     -2.68% (p=0.012 n=10)
  SetGet/2048-4          233.4n ±  1%    574.1n ±  2%   +145.99% (p=0.000 n=10)    366.6n ±  1%    +57.05% (p=0.000 n=10)
  GetParallel/2048-4     31.09n ±  3%   196.50n ±  3%   +532.04% (p=0.000 n=10)    30.42n ±  4%          ~ (p=0.063 n=10)
  SetParallel/2048-4     37.56n ±  7%    66.00n ±  7%    +75.74% (p=0.000 n=10)   182.20n ±  2%   +385.16% (p=0.000 n=10)
  Set/4096-4             223.0n ±  0%    447.0n ±  1%   +100.45% (p=0.000 n=10)    374.0n ±  1%    +67.74% (p=0.000 n=10)
  Get/4096-4             202.4n ±  0%    590.8n ±  2%   +191.87% (p=0.000 n=10)    172.2n ±  0%    -14.95% (p=0.000 n=10)
  SetGet/4096-4          409.3n ±  2%   1126.5n ±  1%   +175.23% (p=0.000 n=10)    482.2n ±  1%    +17.80% (p=0.000 n=10)
  GetParallel/4096-4     63.27n ±  6%   344.25n ±  1%   +444.10% (p=0.000 n=10)    59.99n ±  5%     -5.18% (p=0.015 n=10)
  SetParallel/4096-4     69.07n ±  5%   126.75n ±  4%    +83.51% (p=0.000 n=10)   201.70n ±  4%   +192.02% (p=0.000 n=10)
  Set/8192-4             399.1n ±  1%    892.6n ±  1%   +123.65% (p=0.000 n=10)    526.6n ±  2%    +31.93% (p=0.000 n=10)
  Get/8192-4             384.5n ±  0%   1451.5n ± 24%   +277.50% (p=0.000 n=10)    301.3n ±  1%    -21.63% (p=0.000 n=10)
  SetGet/8192-4          773.7n ±  0%   2471.5n ± 13%   +219.44% (p=0.000 n=10)    678.8n ±  1%    -12.27% (p=0.000 n=10)
  GetParallel/8192-4     112.4n ± 15%    632.0n ± 11%   +462.28% (p=0.000 n=10)    101.3n ±  6%     -9.83% (p=0.000 n=10)
  SetParallel/8192-4     123.6n ±  5%    251.2n ±  5%   +103.36% (p=0.000 n=10)    235.8n ±  5%    +90.81% (p=0.000 n=10)
  geomean                48.52n          118.4n         +143.94%                   116.2n         +139.50%

                     │ fastcache_fork │                 fastcache                 │                 otter                 │
                     │      B/op      │     B/op      vs base                     │     B/op      vs base                 │
  Set/1-4                64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 2%  +1.56% (p=0.000 n=10)
  Get/1-4                0.000 ± 0%       8.000 ± 0%           ? (p=0.000 n=10)       0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/1-4            64.000 ± 0%       8.000 ± 0%     -87.50% (p=0.000 n=10)      66.000 ± 0%  +3.12% (p=0.000 n=10)
  GetParallel/1-4        0.000 ± 0%       8.000 ± 0%           ? (p=0.000 n=10)       0.000 ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/1-4        64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       69.00 ± 0%  +7.81% (p=0.000 n=10)
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
  Set/1024-4             64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       65.00 ± 2%  +1.56% (p=0.000 n=10)
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
  Get/4096-4           0.000Ki ± 0%     3.246Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/4096-4          64.00 ± 0%     4096.00 ± 0%   +6300.00% (p=0.000 n=10)       66.50 ± 1%  +3.91% (p=0.000 n=10)
  GetParallel/4096-4   0.000Ki ± 0%     3.244Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/4096-4     64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  Set/8192-4             64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       67.00 ± 0%  +4.69% (p=0.000 n=10)
  Get/8192-4           0.000Ki ± 0%     6.479Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetGet/8192-4          64.00 ± 0%     8192.00 ± 0%  +12700.00% (p=0.000 n=10)       68.00 ± 1%  +6.25% (p=0.000 n=10)
  GetParallel/8192-4   0.000Ki ± 0%     6.448Ki ± 0%           ? (p=0.000 n=10)     0.000Ki ± 0%       ~ (p=1.000 n=10) ¹
  SetParallel/8192-4     64.00 ± 0%        0.00 ± 0%    -100.00% (p=0.000 n=10)       64.00 ± 0%       ~ (p=1.000 n=10) ¹
  geomean                           ²                 ?                         ²                 +1.46%                ²
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

* **2.4x faster** geomean sec/op than otter and the original fastcache (48.5 ns vs 116.2 ns and 118.4 ns).
* **Zero allocations on `Get`** with string keys. Every `Set` allocates the entry it stores (64 B/op here), about what otter allocates on every write. The original fastcache allocates on reads instead: its `Get` with a `nil` destination copies the value out, so B/op tracks the value size. Its allocs/op shows 0 from `Get/256` up because some of those `Get` calls miss, and the average, below 1, is truncated to an integer.
* **`GetParallel`** is 5x to 11x faster than the original fastcache. Otter still takes 5% to 31% less time in 8 of 10 `GetParallel` sizes, and 1% to 22% less in `Get` from 1024 keys up.
* **Writes**: this fork wins every `Set` and `SetParallel` case against otter. Against the original fastcache it wins every `SetGet` case and `Set` from 128 keys up; the original is faster for `Set` on 1 and 16 keys.

Run benchmarks yourself:

```bash
make bench -C benchmarks
```

## Limitations

* No cache expiration: entries are evicted only when the cache is full (FIFO order).
* No memory accounting: `NewWeighted` bounds the total of the weights that your function returns, not the memory the cache uses.

## Status

> [!CAUTION]
> **`fastcache`** is pre-v1 and does NOT provide a stable API; **use at your own risk**.

Occasional breaking changes may be introduced without notice until a post-v1 release.

## License

MIT. Same as original. See [LICENSE](/LICENSE).
