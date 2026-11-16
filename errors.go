package fastcache

import "errors"

var (
	// ErrInvalidMaxEntries reports an invalid cache capacity.
	ErrInvalidMaxEntries = errors.New("fastcache: maxEntries must be greater than 0")

	// ErrEvictionFailed reports that the cache could not evict an entry while full.
	ErrEvictionFailed = errors.New("fastcache: failed to evict while cache is full")

	// ErrInvalidMaxWeight reports an invalid maximum total weight.
	ErrInvalidMaxWeight = errors.New("fastcache: maxWeight must be greater than 0")

	// ErrInvalidWeight reports a value whose weight is negative or greater
	// than the maximum total weight of the cache.
	ErrInvalidWeight = errors.New("fastcache: weight must be between 0 and maxWeight")

	errNilWeigh = errors.New("fastcache: weigh must not be nil")

	errUnknownOp = errors.New("fastcache: unknown operation")

	errUnsupportedType = errors.New("fastcache: unsupported type for persistence")

	errTrailingData = errors.New("fastcache: saved data continues after the last entry")
)
