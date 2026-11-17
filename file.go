package fastcache

import (
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
// encode, such as an unexported struct field. It cannot detect fields dropped
// from a struct that embeds a type with a GobEncode or MarshalBinary method;
// see [Cache.SaveTo].
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
// not encode, such as an unexported struct field. It cannot detect fields
// dropped from a struct that embeds a type with a GobEncode or MarshalBinary
// method; see [Cache.SaveTo].
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
//
// SaveTo cannot detect one kind of dropped field. A struct that embeds a type
// with a GobEncode or MarshalBinary method, such as [time.Time], inherits that
// method, so gob encodes only the embedded value. SaveTo and [LoadFrom] return
// nil, the struct's other fields load as zero values, and keys that differed
// only in those fields load as one entry. Give the embedded field a name or,
// for values, convert them with [SaveToFunc].
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
// encode, such as an unexported struct field. It cannot detect fields dropped
// from a struct that embeds a type with a GobEncode or MarshalBinary method,
// so neither K nor S should embed one; see [Cache.SaveTo].
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
// To restore a weighted cache, make it with [NewWeighted] and pass its
// [Cache.Set] to [LoadFromFunc]. LoadFromFile drops the weight bound and keeps
// only the saved maxEntries, so a cache saved with [math.MaxInt] as maxEntries
// loads with no bound at all.
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
// evictions. The cache is made by [New], so it has no weight bound. To restore
// a weighted cache, make it with [NewWeighted] and pass its [Cache.Set] to
// [LoadFromFunc].
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

// LoadFrom loads cache data from the given reader. When it succeeds, it has
// read r to the end of the saved data and no further.
//
// To restore a weighted cache, make it with [NewWeighted] and pass its
// [Cache.Set] to [LoadFromFunc]. LoadFrom drops the weight bound and keeps
// only the saved maxEntries, so a cache saved with [math.MaxInt] as maxEntries
// loads with no bound at all.
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
// error, or an error if the data is corrupted. When it returns nil, it has
// read r to the end of the saved data and no further.
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
	stream := &streamReader{r: r, header: [chunkHeaderLen]byte{}, read: 0, left: 0, eof: false}
	dec := gob.NewDecoder(minlz.NewReader(stream))

	var maxEntries int
	if err := dec.Decode(&maxEntries); err != nil {
		return nil, 0, fmt.Errorf("cannot decode maxEntries: %w", err)
	}

	return dec, maxEntries, nil
}

// decodeEntries decodes the entries that follow the capacity and calls load
// for each, in the order they were written, then reads to the end of the
// data. It returns the first error that load returns.
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

	// The minlz stream ends with an EOF chunk after the last entry. Reading
	// it checks the length of the stream, and a SaveTo into a pipe returns
	// only once the chunk has been read. The zero Value discards whatever
	// gob decodes. streamReader returns a plain io.EOF after the chunk, so a
	// wrapped io.EOF means that r ended before it.
	switch err := dec.DecodeValue(reflect.Value{}); {
	case err == nil:
		return errTrailingData
	case err != io.EOF: //nolint:errorlint // See above.
		return fmt.Errorf("cannot read the end of the data: %w", err)
	}

	return nil
}

// A minlz stream is a sequence of chunks, each a type byte, a 3-byte
// little-endian body length and the body. See "STREAM FORMAT" in minlz's
// SPEC.md.
const (
	chunkHeaderLen     = 4
	chunkTypeEOF       = 0x20
	chunkTypeSkippable = 0x40 // the lowest type that a reader may skip
)

// streamReader passes one minlz stream through from r and returns io.EOF
// after the stream's EOF chunk. minlz.Reader reads on after that chunk, for
// a stream that may follow it, so from a pipe or a connection it would wait
// for data that SaveTo never writes. streamReader also rejects the chunk
// types that SaveTo never writes.
type streamReader struct {
	r      io.Reader
	header [chunkHeaderLen]byte
	read   int  // bytes of the current chunk's header read so far
	left   int  // bytes of the current chunk's body not read yet
	eof    bool // the current chunk is the EOF chunk
}

// Read returns the errors of r unwrapped, since minlz.Reader compares them
// with io.EOF.
//
//nolint:wrapcheck // See above.
func (s *streamReader) Read(buf []byte) (int, error) {
	if s.read == len(s.header) && s.left == 0 {
		if s.eof {
			return 0, io.EOF
		}

		s.read = 0
	}

	if s.read < len(s.header) {
		// Stop at the end of the header, so the body's length is known
		// before any of the body is read.
		count, err := s.r.Read(buf[:min(len(buf), len(s.header)-s.read)])

		// From 0x40 up, SaveTo writes only the stream identifier. minlz
		// skips most of those types without a check, so a data chunk whose
		// type byte was damaged into that range would silently go missing,
		// and the entries decoded across the gap would differ from the
		// saved ones.
		if s.read == 0 && count > 0 && buf[0] >= chunkTypeSkippable && buf[0] != minlz.ChunkTypeStreamIdentifier {
			return 0, fmt.Errorf("%w: unexpected chunk type %#x", minlz.ErrCorrupt, buf[0])
		}

		s.read += copy(s.header[s.read:], buf[:count])

		if s.read == len(s.header) {
			// The length is the 3 bytes after the type, little-endian.
			var length [4]byte

			copy(length[:], s.header[1:])
			s.left = int(binary.LittleEndian.Uint32(length[:]))
			s.eof = s.header[0] == chunkTypeEOF
		}

		return count, err
	}

	count, err := s.r.Read(buf[:min(len(buf), s.left)])
	s.left -= count

	return count, err
}
