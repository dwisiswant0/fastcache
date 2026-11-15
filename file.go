package fastcache

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/minio/minlz"
)

// syncFile flushes f to stable storage; tests replace it.
var syncFile = (*os.File).Sync

// SaveToFile atomically saves cache data to the given filePath.
//
// The data is serialized using [gob] and compressed with [minlz].
// SaveToFile may be called concurrently with other ops on the cache.
//
// The saved data may be loaded with [LoadFromFile].
//
// SaveToFile returns an error if K or V has a field that gob would not
// encode, such as an unexported struct field.
func (c *Cache[K, V]) SaveToFile(filePath string) error {
	return c.SaveToFileConcurrent(filePath, 1)
}

// SaveToFileConcurrent saves cache data to the given filePath using
// the specified number of concurrent workers.
//
// SaveToFileConcurrent may be called concurrently with other ops on the cache.
//
// The saved data may be loaded with [LoadFromFile].
//
// SaveToFileConcurrent returns an error if K or V has a field that gob would
// not encode, such as an unexported struct field.
func (c *Cache[K, V]) SaveToFileConcurrent(filePath string, concurrency int) error {
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("cannot stat %q: %w", dir, err)
		}

		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create dir %q: %w", dir, err)
		}
	}

	// Save cache data into a temporary file.
	tmpFile, err := os.CreateTemp(dir, "fastcache.tmp.*")
	if err != nil {
		return fmt.Errorf("cannot create temporary file in %q: %w", dir, err)
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

		return fmt.Errorf("cannot save cache data to %q: %w", tmpPath, err)
	}

	if err := syncFile(tmpFile); err != nil {
		_ = tmpFile.Close()

		return fmt.Errorf("cannot sync temporary file %q: %w", tmpPath, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("cannot rename %q to %q: %w", tmpPath, filePath, err)
	}

	return nil
}

// SaveTo saves cache data to the given writer.
//
// The data is serialized using [gob] and compressed with [minlz].
// SaveTo may be called concurrently with other ops on the cache.
//
// The saved data may be loaded with [LoadFrom] or [LoadFromFunc].
//
// SaveTo returns an error if K or V has a field that gob would not encode,
// such as an unexported struct field.
func (c *Cache[K, V]) SaveTo(w io.Writer) error {
	return c.save(w, 1)
}

// SaveToFunc saves to w the entries of c for which save reports true, oldest
// first, with the values that save returns in place of their own. Use it to
// save only some entries, or to save values of a type that gob cannot encode,
// such as a struct with unexported fields, as values of a type it can.
//
// The data is serialized using [gob] and compressed with [minlz], as with
// [Cache.SaveTo]. It may be loaded with [LoadFrom] or [LoadFromFunc] as
// values of type S. SaveToFunc may be called concurrently with other ops on
// the cache.
//
// SaveToFunc returns an error if K or S has a field that gob would not
// encode, such as an unexported struct field.
func SaveToFunc[K comparable, V, S any](c *Cache[K, V], w io.Writer, save func(K, V) (S, bool)) error {
	if err := checkGobTypes[K, S](); err != nil {
		return err
	}

	var saved []entry[K, S]

	for _, e := range c.entriesOldestFirst(1) {
		if s, ok := save(e.Key, e.Value); ok {
			saved = append(saved, entry[K, S]{Key: e.Key, Value: s})
		}
	}

	return encode(w, c.maxEntries, saved)
}

func (c *Cache[K, V]) save(w io.Writer, concurrency int) error {
	// gob silently drops fields it does not send, so reject such types
	// before writing anything instead of saving data that loads back wrong.
	if err := checkGobTypes[K, V](); err != nil {
		return err
	}

	return encode(w, c.maxEntries, c.entriesOldestFirst(concurrency))
}

// encode writes the capacity of the saved cache, the number of entries, and
// the entries, each an entry or a pointer to one. Loading inserts the entries
// in the order they are written, so they must come oldest first to keep the
// FIFO eviction order.
func encode[E any](w io.Writer, maxEntries int, entries []E) error {
	zw := minlz.NewWriter(w)
	// zw runs a goroutine until it is closed, so the error paths must close
	// it too. On success, the Close below reports its error.
	defer func() {
		_ = zw.Close()
	}()

	enc := gob.NewEncoder(zw)

	if err := enc.Encode(maxEntries); err != nil {
		return fmt.Errorf("cannot encode maxEntries: %w", err)
	}

	if err := enc.Encode(len(entries)); err != nil {
		return fmt.Errorf("cannot encode entry count: %w", err)
	}

	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("cannot encode entry: %w", err)
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("cannot close minlz writer: %w", err)
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

// LoadFromFileOrNew returns a new cache with the given maxEntries capacity,
// filled with the entries saved in filePath, oldest first. It ignores the
// capacity of the saved cache: when the file holds more than maxEntries
// entries, the cache keeps the newest, and its stats count the others as
// evictions.
//
// If the file does not exist or cannot be loaded, the cache is returned empty.
// LoadFromFileOrNew returns an error only if maxEntries is not positive.
func LoadFromFileOrNew[K comparable, V any](filePath string, maxEntries int) (*Cache[K, V], error) {
	c, err := New[K, V](maxEntries)
	if err != nil {
		return nil, err
	}

	if f, err := os.Open(filePath); err == nil {
		defer func() {
			_ = f.Close()
		}()

		// Drop the entries loaded before the error, so a corrupted file
		// gives the same empty cache as a missing one.
		if err := LoadFromFunc(f, c.Set); err != nil {
			c.Reset()
		}
	}

	return c, nil
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
	dec, maxEntries, err := newDecoder(r)
	if err != nil {
		return nil, err
	}

	c, err := New[K, V](maxEntries)
	if err != nil {
		return nil, fmt.Errorf("cannot create cache: %w", err)
	}

	if err := decodeEntries(dec, c.Set); err != nil {
		return nil, err
	}

	return c, nil
}

// LoadFromFunc calls load with the key and value of each entry saved in r,
// oldest first, and stops at the first error load returns. It returns that
// error, or an error if the data is corrupted.
//
// LoadFromFunc reads the data that [Cache.SaveTo], [Cache.SaveToFile] and
// [SaveToFunc] write, and S must be a type that gob can decode the saved
// values into. It ignores the capacity of the saved cache: to load the entries
// into a cache c of any capacity, pass c.Set as load. An empty c then keeps the
// newest entries when the data holds more than it can. When LoadFromFunc fails
// partway, the entries before the failure have already been passed to load.
func LoadFromFunc[K comparable, S any](r io.Reader, load func(K, S) error) error {
	dec, _, err := newDecoder(r)
	if err != nil {
		return err
	}

	return decodeEntries(dec, load)
}

// unexpectedEOF returns io.ErrUnexpectedEOF in place of io.EOF. After the
// capacity, the data must still hold the entry count and every entry that it
// promises, so data that ends before them was cut short.
func unexpectedEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}

	return err
}

// newDecoder returns a decoder for the data that encode wrote to r, and the
// capacity of the saved cache, which the data starts with.
func newDecoder(r io.Reader) (*gob.Decoder, int, error) {
	dec := gob.NewDecoder(minlz.NewReader(r))

	var maxEntries int
	if err := dec.Decode(&maxEntries); err != nil {
		return nil, 0, fmt.Errorf("cannot decode maxEntries: %w", err)
	}

	return dec, maxEntries, nil
}

// decodeEntries decodes the entries that follow the capacity and calls load
// for each, in the order they were written. It returns the first error that
// load returns.
func decodeEntries[K comparable, S any](dec *gob.Decoder, load func(K, S) error) error {
	var count int
	if err := dec.Decode(&count); err != nil {
		return fmt.Errorf("cannot decode entry count: %w", unexpectedEOF(err))
	}

	for i := range count {
		var e entry[K, S]
		if err := dec.Decode(&e); err != nil {
			return fmt.Errorf("cannot decode entry %d: %w", i, unexpectedEOF(err))
		}

		if err := load(e.Key, e.Value); err != nil {
			return err
		}
	}

	return nil
}
