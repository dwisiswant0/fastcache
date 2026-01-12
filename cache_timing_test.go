package fastcache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkCacheSet(b *testing.B) {
	c := New[string, string](b.N * 2)
	defer c.Reset()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		c.Set(k, v)
	}
}

func BenchmarkCacheGet(b *testing.B) {
	c := New[string, string](b.N * 2)
	defer c.Reset()

	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		c.Set(k, v)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		c.Get(k)
	}
}

func BenchmarkCacheSetGet(b *testing.B) {
	c := New[string, string](b.N * 2)
	defer c.Reset()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		c.Set(k, v)
		c.Get(k)
	}
}

func BenchmarkCacheSetGetConcurrent(b *testing.B) {
	c := New[string, string](b.N * 2)
	defer c.Reset()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			k := fmt.Sprintf("key %d", i)
			v := fmt.Sprintf("value %d", i)
			c.Set(k, v)
			c.Get(k)
			i++
		}
	})
}

func BenchmarkCacheGetConcurrent(b *testing.B) {
	c := New[string, string](1000000)
	defer c.Reset()

	// Pre-populate
	for i := 0; i < 1000000; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		c.Set(k, v)
	}

	var counter atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := counter.Add(1) % 1000000
			k := fmt.Sprintf("key %d", i)
			c.Get(k)
		}
	})
}

func BenchmarkCacheSetBytes(b *testing.B) {
	c := New[string, []byte](b.N * 2)
	defer c.Reset()

	value := make([]byte, 100)
	for i := range value {
		value[i] = byte(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		c.Set(k, value)
	}
}

func BenchmarkCacheGetBytes(b *testing.B) {
	c := New[string, []byte](b.N * 2)
	defer c.Reset()

	value := make([]byte, 100)
	for i := range value {
		value[i] = byte(i)
	}

	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		c.Set(k, value)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		c.Get(k)
	}
}

func BenchmarkCacheIntKey(b *testing.B) {
	c := New[int, string](b.N * 2)
	defer c.Reset()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(i, "value")
		c.Get(i)
	}
}

func BenchmarkCacheUint64Key(b *testing.B) {
	c := New[uint64, string](b.N * 2)
	defer c.Reset()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(uint64(i), "value")
		c.Get(uint64(i))
	}
}

func BenchmarkMapSetGet(b *testing.B) {
	m := make(map[string]string, b.N)
	var mu sync.RWMutex

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		mu.Lock()
		m[k] = v
		mu.Unlock()
		mu.RLock()
		_ = m[k]
		mu.RUnlock()
	}
}

func BenchmarkMapSetGetConcurrent(b *testing.B) {
	m := make(map[string]string, b.N)
	var mu sync.RWMutex

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			k := fmt.Sprintf("key %d", i)
			v := fmt.Sprintf("value %d", i)
			mu.Lock()
			m[k] = v
			mu.Unlock()
			mu.RLock()
			_ = m[k]
			mu.RUnlock()
			i++
		}
	})
}

func BenchmarkSyncMapSetGet(b *testing.B) {
	var m sync.Map

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		m.Store(k, v)
		m.Load(k)
	}
}

func BenchmarkSyncMapSetGetConcurrent(b *testing.B) {
	var m sync.Map

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			k := fmt.Sprintf("key %d", i)
			v := fmt.Sprintf("value %d", i)
			m.Store(k, v)
			m.Load(k)
			i++
		}
	})
}
