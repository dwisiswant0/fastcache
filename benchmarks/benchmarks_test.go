package benchmarks_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	original "github.com/VictoriaMetrics/fastcache"
	"github.com/maypok86/otter/v2"
	"go.dw1.io/fastcache"
)

var sizes = []int{1, 16, 32, 128, 256, 512, 1024, 2048, 4096, 8192}

// Lazily generated keys and values per size so CI doesn't preallocate everything at once.
type testData struct {
	keys   [][]byte
	values [][]byte
}

type dataEntry struct {
	once sync.Once
	data *testData
}

var (
	dataMu     sync.Mutex
	dataBySize = make(map[int]*dataEntry)
)

func getData(size int) *testData {
	dataMu.Lock()
	entry, ok := dataBySize[size]
	if !ok {
		entry = &dataEntry{}
		dataBySize[size] = entry
	}
	dataMu.Unlock()

	entry.once.Do(func() {
		maxItems := 12 * size
		d := &testData{
			keys:   make([][]byte, maxItems),
			values: make([][]byte, maxItems),
		}
		for i := range maxItems {
			d.keys[i] = make([]byte, size)
			d.values[i] = make([]byte, size)
			for j := 0; j < size; j++ {
				d.keys[i][j] = byte(i>>8) ^ byte(j)
				d.values[i][j] = byte(i) ^ byte(j)
			}
		}
		entry.data = d
	})

	return entry.data
}

// ============================================================================
// Original Fastcache (github.com/VictoriaMetrics/fastcache)
// ============================================================================

func BenchmarkFastcache(b *testing.B) {
	for _, size := range sizes {
		maxItems := 12 * size
		data := getData(size)

		b.Run(fmt.Sprintf("Set/%d", size), func(b *testing.B) {
			c := original.New(maxItems)
			defer c.Reset()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.Set(data.keys[idx], data.values[idx])
			}
		})

		b.Run(fmt.Sprintf("Get/%d", size), func(b *testing.B) {
			c := original.New(maxItems)
			defer c.Reset()

			// Pre-populate
			for i := range maxItems {
				c.Set(data.keys[i], data.values[i])
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.Get(nil, data.keys[idx])
			}
		})

		b.Run(fmt.Sprintf("SetGet/%d", size), func(b *testing.B) {
			c := original.New(maxItems)
			defer c.Reset()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.Set(data.keys[idx], data.values[idx])
				c.Get(nil, data.keys[idx])
			}
		})

		b.Run(fmt.Sprintf("GetParallel/%d", size), func(b *testing.B) {
			c := original.New(maxItems)
			defer c.Reset()

			// Pre-populate
			for i := range maxItems {
				c.Set(data.keys[i], data.values[i])
			}

			var start atomic.Int64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				idx := int(start.Add(1)-1) % maxItems
				for pb.Next() {
					c.Get(nil, data.keys[idx])
					idx++
					if idx == maxItems {
						idx = 0
					}
				}
			})
		})

		b.Run(fmt.Sprintf("SetParallel/%d", size), func(b *testing.B) {
			c := original.New(maxItems)
			defer c.Reset()

			var start atomic.Int64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				idx := int(start.Add(1)-1) % maxItems
				for pb.Next() {
					c.Set(data.keys[idx], data.values[idx])
					idx++
					if idx == maxItems {
						idx = 0
					}
				}
			})
		})
	}
}

// ============================================================================
// Fork Fastcache (go.dw1.io/fastcache) - Generic version
// ============================================================================

func BenchmarkFastcacheFork(b *testing.B) {
	for _, size := range sizes {
		maxItems := 12 * size
		data := getData(size)

		// Convert to string keys for the generic cache
		stringKeys := make([]string, maxItems)
		for i := range maxItems {
			stringKeys[i] = string(data.keys[i])
		}

		b.Run(fmt.Sprintf("Set/%d", size), func(b *testing.B) {
			c := fastcache.New[string, []byte](12 * size)
			defer c.Reset()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.Set(stringKeys[idx], data.values[idx])
			}
		})

		b.Run(fmt.Sprintf("Get/%d", size), func(b *testing.B) {
			c := fastcache.New[string, []byte](12 * size)
			defer c.Reset()

			// Pre-populate
			for i := range maxItems {
				c.Set(stringKeys[i], data.values[i])
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.Get(stringKeys[idx])
			}
		})

		b.Run(fmt.Sprintf("SetGet/%d", size), func(b *testing.B) {
			c := fastcache.New[string, []byte](12 * size)
			defer c.Reset()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.Set(stringKeys[idx], data.values[idx])
				c.Get(stringKeys[idx])
			}
		})

		b.Run(fmt.Sprintf("GetParallel/%d", size), func(b *testing.B) {
			c := fastcache.New[string, []byte](12 * size)
			defer c.Reset()

			// Pre-populate
			for i := range maxItems {
				c.Set(stringKeys[i], data.values[i])
			}

			var start atomic.Int64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				idx := int(start.Add(1)-1) % maxItems
				for pb.Next() {
					c.Get(stringKeys[idx])
					idx++
					if idx == maxItems {
						idx = 0
					}
				}
			})
		})

		b.Run(fmt.Sprintf("SetParallel/%d", size), func(b *testing.B) {
			c := fastcache.New[string, []byte](12 * size)
			defer c.Reset()

			var start atomic.Int64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				idx := int(start.Add(1)-1) % maxItems
				for pb.Next() {
					c.Set(stringKeys[idx], data.values[idx])
					idx++
					if idx == maxItems {
						idx = 0
					}
				}
			})
		})
	}
}

// ============================================================================
// Otter (github.com/maypok86/otter/v2)
// ============================================================================

func BenchmarkOtter(b *testing.B) {
	for _, size := range sizes {
		maxItems := 12 * size
		data := getData(size)

		// Convert to string keys for otter
		stringKeys := make([]string, maxItems)
		for i := range maxItems {
			stringKeys[i] = string(data.keys[i])
		}

		b.Run(fmt.Sprintf("Set/%d", size), func(b *testing.B) {
			c := otter.Must(&otter.Options[string, []byte]{
				MaximumSize: 12 * size,
			})

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.Set(stringKeys[idx], data.values[idx])
			}
		})

		b.Run(fmt.Sprintf("Get/%d", size), func(b *testing.B) {
			c := otter.Must(&otter.Options[string, []byte]{
				MaximumSize: 12 * size,
			})

			// Pre-populate
			for i := range maxItems {
				c.Set(stringKeys[i], data.values[i])
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.GetIfPresent(stringKeys[idx])
			}
		})

		b.Run(fmt.Sprintf("SetGet/%d", size), func(b *testing.B) {
			c := otter.Must(&otter.Options[string, []byte]{
				MaximumSize: 12 * size,
			})

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx := i % maxItems
				c.Set(stringKeys[idx], data.values[idx])
				c.GetIfPresent(stringKeys[idx])
			}
		})

		b.Run(fmt.Sprintf("GetParallel/%d", size), func(b *testing.B) {
			c := otter.Must(&otter.Options[string, []byte]{
				MaximumSize: 12 * size,
			})

			// Pre-populate
			for i := range maxItems {
				c.Set(stringKeys[i], data.values[i])
			}

			var start atomic.Int64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				idx := int(start.Add(1)-1) % maxItems
				for pb.Next() {
					c.GetIfPresent(stringKeys[idx])
					idx++
					if idx == maxItems {
						idx = 0
					}
				}
			})
		})

		b.Run(fmt.Sprintf("SetParallel/%d", size), func(b *testing.B) {
			c := otter.Must(&otter.Options[string, []byte]{
				MaximumSize: 12 * size,
			})

			var start atomic.Int64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				idx := int(start.Add(1)-1) % maxItems
				for pb.Next() {
					c.Set(stringKeys[idx], data.values[idx])
					idx++
					if idx == maxItems {
						idx = 0
					}
				}
			})
		})
	}
}
