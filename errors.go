package fastcache

import "errors"

var (
	// ErrInvalidMaxEntries reports an invalid cache capacity.
	ErrInvalidMaxEntries = errors.New("fastcache: maxEntries must be greater than 0")

	// ErrEvictionFailed reports that the cache could not evict an entry while full.
	ErrEvictionFailed = errors.New("fastcache: failed to evict while cache is full")

	errUnknownOp = errors.New("fastcache: unknown operation")
)
