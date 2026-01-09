package fastcache

import (
	"fmt"
	"sync"
	"testing"
)

func BenchmarkCacheSet(b *testing.B) {
	const items = 1 << 16
	c := New(12 * items)
	defer c.Reset()
	b.ReportAllocs()
	b.SetBytes(items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		v := []byte("xyza")
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				c.Set(k, v)
			}
		}
	})
}

func BenchmarkCacheGet(b *testing.B) {
	const items = 1 << 16
	c := New(12 * items)
	defer c.Reset()
	k := []byte("\x00\x00\x00\x00")
	v := []byte("xyza")
	for i := 0; i < items; i++ {
		k[0]++
		if k[0] == 0 {
			k[1]++
		}
		c.Set(k, v)
	}

	b.ReportAllocs()
	b.SetBytes(items)
	b.RunParallel(func(pb *testing.PB) {
		var buf []byte
		k := []byte("\x00\x00\x00\x00")
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				buf = c.Get(buf[:0], k)
				if string(buf) != string(v) {
					panic(fmt.Errorf("BUG: invalid value obtained; got %q; want %q", buf, v))
				}
			}
		}
	})
}

func BenchmarkCacheHas(b *testing.B) {
	const items = 1 << 16
	c := New(12 * items)
	defer c.Reset()
	k := []byte("\x00\x00\x00\x00")
	for i := 0; i < items; i++ {
		k[0]++
		if k[0] == 0 {
			k[1]++
		}
		c.Set(k, nil)
	}

	b.ReportAllocs()
	b.SetBytes(items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				if !c.Has(k) {
					panic(fmt.Errorf("BUG: missing value for key %q", k))
				}
			}
		}
	})
}

func BenchmarkCacheSetGet(b *testing.B) {
	const items = 1 << 16
	c := New(12 * items)
	defer c.Reset()
	b.ReportAllocs()
	b.SetBytes(2 * items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		v := []byte("xyza")
		var buf []byte
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				c.Set(k, v)
			}
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				buf = c.Get(buf[:0], k)
				if string(buf) != string(v) {
					panic(fmt.Errorf("BUG: invalid value obtained; got %q; want %q", buf, v))
				}
			}
		}
	})
}

func BenchmarkStdMapSet(b *testing.B) {
	const items = 1 << 16
	m := make(map[string][]byte)
	var mu sync.Mutex
	b.ReportAllocs()
	b.SetBytes(items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		v := []byte("xyza")
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				mu.Lock()
				m[string(k)] = v
				mu.Unlock()
			}
		}
	})
}

func BenchmarkStdMapGet(b *testing.B) {
	const items = 1 << 16
	m := make(map[string][]byte)
	k := []byte("\x00\x00\x00\x00")
	v := []byte("xyza")
	for i := 0; i < items; i++ {
		k[0]++
		if k[0] == 0 {
			k[1]++
		}
		m[string(k)] = v
	}

	var mu sync.RWMutex
	b.ReportAllocs()
	b.SetBytes(items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				mu.RLock()
				vv := m[string(k)]
				mu.RUnlock()
				if string(vv) != string(v) {
					panic(fmt.Errorf("BUG: unexpected value; got %q; want %q", vv, v))
				}
			}
		}
	})
}

func BenchmarkStdMapSetGet(b *testing.B) {
	const items = 1 << 16
	m := make(map[string][]byte)
	var mu sync.RWMutex
	b.ReportAllocs()
	b.SetBytes(2 * items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		v := []byte("xyza")
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				mu.Lock()
				m[string(k)] = v
				mu.Unlock()
			}
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				mu.RLock()
				vv := m[string(k)]
				mu.RUnlock()
				if string(vv) != string(v) {
					panic(fmt.Errorf("BUG: unexpected value; got %q; want %q", vv, v))
				}
			}
		}
	})
}

func BenchmarkSyncMapSet(b *testing.B) {
	const items = 1 << 16
	m := sync.Map{}
	b.ReportAllocs()
	b.SetBytes(items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		v := "xyza"
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				m.Store(string(k), v)
			}
		}
	})
}

func BenchmarkSyncMapGet(b *testing.B) {
	const items = 1 << 16
	m := sync.Map{}
	k := []byte("\x00\x00\x00\x00")
	v := "xyza"
	for i := 0; i < items; i++ {
		k[0]++
		if k[0] == 0 {
			k[1]++
		}
		m.Store(string(k), v)
	}

	b.ReportAllocs()
	b.SetBytes(items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				vv, ok := m.Load(string(k))
				if !ok || vv.(string) != string(v) {
					panic(fmt.Errorf("BUG: unexpected value; got %q; want %q", vv, v))
				}
			}
		}
	})
}

func BenchmarkSyncMapSetGet(b *testing.B) {
	const items = 1 << 16
	m := sync.Map{}
	b.ReportAllocs()
	b.SetBytes(2 * items)
	b.RunParallel(func(pb *testing.PB) {
		k := []byte("\x00\x00\x00\x00")
		v := "xyza"
		for pb.Next() {
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				m.Store(string(k), v)
			}
			for i := 0; i < items; i++ {
				k[0]++
				if k[0] == 0 {
					k[1]++
				}
				vv, ok := m.Load(string(k))
				if !ok || vv.(string) != string(v) {
					panic(fmt.Errorf("BUG: unexpected value; got %q; want %q", vv, v))
				}
			}
		}
	})
}
