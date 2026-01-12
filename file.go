package fastcache

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/golang/snappy"
)

// SaveToFile atomically saves cache data to the given filePath.
//
// The data is serialized using [gob] and compressed with [snappy].
// SaveToFile may be called concurrently with other ops on the cache.
//
// The saved data may be loaded with [LoadFromFile].
func (c *Cache[K, V]) SaveToFile(filePath string) error {
	return c.SaveToFileConcurrent(filePath, 1)
}

// SaveToFileConcurrent saves cache data to the given filePath using
// the specified number of concurrent workers.
//
// SaveToFileConcurrent may be called concurrently with other ops on the cache.
//
// The saved data may be loaded with [LoadFromFile].
func (c *Cache[K, V]) SaveToFileConcurrent(filePath string, concurrency int) error {
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("cannot stat %q: %s", dir, err)
		}

		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create dir %q: %s", dir, err)
		}
	}

	// Save cache data into a temporary file.
	tmpFile, err := os.CreateTemp(dir, "fastcache.tmp.*")
	if err != nil {
		return fmt.Errorf("cannot create temporary file in %q: %s", dir, err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	gomaxprocs := runtime.GOMAXPROCS(-1)
	if concurrency <= 0 || concurrency > gomaxprocs {
		concurrency = gomaxprocs
	}

	if err := c.save(tmpFile, concurrency); err != nil {
		_ = tmpFile.Close()

		return fmt.Errorf("cannot save cache data to %q: %s", tmpPath, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary file %q: %s", tmpPath, err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("cannot rename %q to %q: %s", tmpPath, filePath, err)
	}

	return nil
}

// SaveTo saves cache data to the given writer.
//
// The data is serialized using [gob] and compressed with [snappy].
// SaveTo may be called concurrently with other ops on the cache.
//
// The saved data may be loaded with [LoadFrom].
func (c *Cache[K, V]) SaveTo(w io.Writer) error {
	return c.save(w, 1)
}

// entry is used for serializing key-value pairs.
type entry[K comparable, V any] struct {
	Key   K
	Value V
}

func (c *Cache[K, V]) save(w io.Writer, concurrency int) error {
	zw := snappy.NewBufferedWriter(w)
	enc := gob.NewEncoder(zw)

	if err := enc.Encode(c.maxEntries); err != nil {
		return fmt.Errorf("cannot encode maxEntries: %s", err)
	}

	type shardData struct {
		idx     int
		entries []entry[K, V]
	}

	shardCh := make(chan int, shardsCount)
	resultCh := make(chan shardData, shardsCount)
	errCh := make(chan error, concurrency)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range shardCh {
				shard := &c.shards[idx]
				shard.mu.RLock()
				entries := make([]entry[K, V], 0, len(shard.entries))
				for k, v := range shard.entries {
					entries = append(entries, entry[K, V]{Key: k, Value: v})
				}
				shard.mu.RUnlock()
				resultCh <- shardData{idx: idx, entries: entries}
			}
		}()
	}

	go func() {
		for i := 0; i < shardsCount; i++ {
			shardCh <- i
		}
		close(shardCh)
		wg.Wait()
		close(resultCh)
	}()

	shardEntries := make([][]entry[K, V], shardsCount)
	for data := range resultCh {
		shardEntries[data.idx] = data.entries
	}

	select {
	case err := <-errCh:
		return err
	default:
	}

	totalEntries := 0
	for _, entries := range shardEntries {
		totalEntries += len(entries)
	}

	if err := enc.Encode(totalEntries); err != nil {
		return fmt.Errorf("cannot encode entry count: %s", err)
	}

	for _, entries := range shardEntries {
		for _, e := range entries {
			if err := enc.Encode(e); err != nil {
				return fmt.Errorf("cannot encode entry: %s", err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("cannot close snappy writer: %s", err)
	}

	return nil
}

// LoadFromFile loads cache data from the given filePath.
//
// Returns an error if the file does not exist or is corrupted.
//
// See [Cache.SaveToFile] for saving cache data to file.
func LoadFromFile[K comparable, V any](filePath string) (*Cache[K, V], error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	return load[K, V](f)
}

// LoadFromFileOrNew tries loading cache data from the given filePath.
//
// The function falls back to creating a new cache with the given maxEntries
// capacity if an error occurs during loading.
func LoadFromFileOrNew[K comparable, V any](filePath string, maxEntries int) *Cache[K, V] {
	c, err := LoadFromFile[K, V](filePath)
	if err == nil {
		return c
	}

	return New[K, V](maxEntries)
}

// LoadFrom loads cache data from the given reader.
//
// Returns an error if the data is corrupted.
//
// See [Cache.SaveTo] for saving cache data to a writer.
func LoadFrom[K comparable, V any](r io.Reader) (*Cache[K, V], error) {
	return load[K, V](r)
}

func load[K comparable, V any](r io.Reader) (*Cache[K, V], error) {
	zr := snappy.NewReader(r)
	dec := gob.NewDecoder(zr)

	var maxEntries int
	if err := dec.Decode(&maxEntries); err != nil {
		return nil, fmt.Errorf("cannot decode maxEntries: %s", err)
	}

	c := New[K, V](maxEntries)

	var totalEntries int
	if err := dec.Decode(&totalEntries); err != nil {
		return nil, fmt.Errorf("cannot decode entry count: %s", err)
	}

	for i := 0; i < totalEntries; i++ {
		var e entry[K, V]
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("cannot decode entry %d: %s", i, err)
		}
		c.Set(e.Key, e.Value)
	}

	return c, nil
}
