package fastcache_test

import (
	"fmt"
	"sort"

	"go.dw1.io/fastcache"
)

// ExampleCache demonstrates basic cache operations.
func ExampleCache() {
	// Create a new cache with capacity for 100 entries
	cache, err := fastcache.New[string, string](100)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Set a key-value pair
	if err := cache.Set("key1", "value1"); err != nil {
		return
	}

	// Get the value
	if value, ok := cache.Get("key1"); ok {
		fmt.Println("Found:", value)
	}

	// Check if a key exists
	if cache.Has("key1") {
		fmt.Println("Key exists")
	}

	// Output:
	// Found: value1
	// Key exists
}

// ExampleCache_GetOrSet demonstrates the GetOrSet method.
func ExampleCache_GetOrSet() {
	cache, err := fastcache.New[string, int](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Key doesn't exist, so it will be set
	value, loaded, err := cache.GetOrSet("counter", 1)
	if err != nil {
		return
	}
	fmt.Printf("First call - Value: %d, Loaded: %t\n", value, loaded)

	// Key exists, so existing value is returned
	value, loaded, err = cache.GetOrSet("counter", 2)
	if err != nil {
		return
	}
	fmt.Printf("Second call - Value: %d, Loaded: %t\n", value, loaded)

	// Output:
	// First call - Value: 1, Loaded: false
	// Second call - Value: 1, Loaded: true
}

// ExampleCache_SetIfAbsent demonstrates the SetIfAbsent method.
func ExampleCache_SetIfAbsent() {
	cache, err := fastcache.New[string, string](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Key doesn't exist, so it will be set
	stored, err := cache.SetIfAbsent("key", "first")
	if err != nil {
		return
	}
	fmt.Printf("First set - Stored: %t\n", stored)

	// Key exists, so it won't be set
	stored, err = cache.SetIfAbsent("key", "second")
	if err != nil {
		return
	}
	fmt.Printf("Second set - Stored: %t\n", stored)

	// Check the value
	if value, ok := cache.Get("key"); ok {
		fmt.Println("Final value:", value)
	}

	// Output:
	// First set - Stored: true
	// Second set - Stored: false
	// Final value: first
}

// ExampleCache_GetAndDelete demonstrates the GetAndDelete method.
func ExampleCache_GetAndDelete() {
	cache, err := fastcache.New[string, string](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	if err := cache.Set("temp", "data"); err != nil {
		return
	}

	// Get and delete the value
	value, loaded := cache.GetAndDelete("temp")
	fmt.Printf("Deleted value: %s, Was loaded: %t\n", value, loaded)

	// Try to get it again - should not exist
	if _, ok := cache.Get("temp"); !ok {
		fmt.Println("Key no longer exists")
	}

	// Output:
	// Deleted value: data, Was loaded: true
	// Key no longer exists
}

// ExampleCache_All demonstrates iterating over all key-value pairs.
func ExampleCache_All() {
	cache, err := fastcache.New[string, int](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Add some data
	if err := cache.Set("item1", 42); err != nil {
		return
	}
	if err := cache.Set("item2", 84); err != nil {
		return
	}

	fmt.Println("Cache entries:")
	entries := make([]string, 0, 2)
	for key, value := range cache.All() {
		entries = append(entries, fmt.Sprintf("%s:%d", key, value))
	}
	sort.Strings(entries)
	for _, entry := range entries {
		fmt.Println(entry)
	}

	// Output:
	// Cache entries:
	// item1:42
	// item2:84
}

// ExampleCache_Keys demonstrates iterating over all keys.
func ExampleCache_Keys() {
	cache, err := fastcache.New[string, int](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	if err := cache.Set("x", 10); err != nil {
		return
	}
	if err := cache.Set("y", 20); err != nil {
		return
	}
	if err := cache.Set("z", 30); err != nil {
		return
	}

	fmt.Println("Keys found:")
	keys := make([]string, 0, 3)
	for key := range cache.Keys() {
		keys = append(keys, key)
	}
	// Sort for consistent output
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Println(key)
	}

	// Output:
	// Keys found:
	// x
	// y
	// z
}

// ExampleCache_Values demonstrates iterating over all values.
func ExampleCache_Values() {
	cache, err := fastcache.New[string, int](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	if err := cache.Set("p", 100); err != nil {
		return
	}
	if err := cache.Set("q", 200); err != nil {
		return
	}
	if err := cache.Set("r", 300); err != nil {
		return
	}

	fmt.Println("Values found:")
	values := make([]int, 0, 3)
	for value := range cache.Values() {
		values = append(values, value)
	}
	// Sort for consistent output
	sort.Ints(values)
	for _, value := range values {
		fmt.Println(value)
	}

	// Output:
	// Values found:
	// 100
	// 200
	// 300
}

// ExampleCache_Len demonstrates getting the cache size.
func ExampleCache_Len() {
	cache, err := fastcache.New[string, string](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	fmt.Println("Initial length:", cache.Len())

	if err := cache.Set("key1", "value1"); err != nil {
		return
	}
	if err := cache.Set("key2", "value2"); err != nil {
		return
	}

	fmt.Println("After adding items:", cache.Len())

	// Output:
	// Initial length: 0
	// After adding items: 2
}

// ExampleCache_Delete demonstrates deleting items from the cache.
func ExampleCache_Delete() {
	cache, err := fastcache.New[string, string](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Add some items
	if err := cache.Set("key1", "value1"); err != nil {
		return
	}
	if err := cache.Set("key2", "value2"); err != nil {
		return
	}

	fmt.Println("Before deletion:")
	fmt.Printf("Length: %d\n", cache.Len())
	if _, ok := cache.Get("key1"); ok {
		fmt.Println("key1 exists")
	}

	// Delete an item
	cache.Delete("key1")

	fmt.Println("After deletion:")
	fmt.Printf("Length: %d\n", cache.Len())
	if _, ok := cache.Get("key1"); !ok {
		fmt.Println("key1 no longer exists")
	}

	// Output:
	// Before deletion:
	// Length: 2
	// key1 exists
	// After deletion:
	// Length: 1
	// key1 no longer exists
}

// ExampleCache_Reset demonstrates resetting the cache.
func ExampleCache_Reset() {
	cache, err := fastcache.New[string, int](10)
	if err != nil {
		return
	}

	// Add some items
	if err := cache.Set("a", 1); err != nil {
		return
	}
	if err := cache.Set("b", 2); err != nil {
		return
	}
	if err := cache.Set("c", 3); err != nil {
		return
	}

	fmt.Printf("Before reset - Length: %d\n", cache.Len())

	// Reset the cache
	cache.Reset()

	fmt.Printf("After reset - Length: %d\n", cache.Len())

	// Verify items are gone
	if _, ok := cache.Get("a"); !ok {
		fmt.Println("Cache is empty after reset")
	}

	// Output:
	// Before reset - Length: 3
	// After reset - Length: 0
	// Cache is empty after reset
}

// ExampleNew demonstrates creating a new cache.
func ExampleNew() {
	// Create a new cache with capacity for 100 entries
	cache, err := fastcache.New[string, int](100)
	if err != nil {
		return
	}
	defer cache.Reset()

	fmt.Printf("Created cache with capacity: %d\n", 100)
	fmt.Printf("Initial length: %d\n", cache.Len())

	// Output:
	// Created cache with capacity: 100
	// Initial length: 0
}

// ExampleCache_Get demonstrates getting values from the cache.
func ExampleCache_Get() {
	cache, err := fastcache.New[string, string](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Set some values
	if err := cache.Set("name", "Alice"); err != nil {
		return
	}
	if err := cache.Set("age", "30"); err != nil {
		return
	}

	// Get existing value
	if value, ok := cache.Get("name"); ok {
		fmt.Printf("Name: %s\n", value)
	}

	// Try to get non-existent value
	if _, ok := cache.Get("city"); !ok {
		fmt.Println("City not found")
	}

	// Output:
	// Name: Alice
	// City not found
}

// ExampleCache_Has demonstrates checking if keys exist in the cache.
func ExampleCache_Has() {
	cache, err := fastcache.New[string, string](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	if err := cache.Set("user", "john"); err != nil {
		return
	}

	// Check existing key
	if cache.Has("user") {
		fmt.Println("User exists")
	}

	// Check non-existent key
	if !cache.Has("admin") {
		fmt.Println("Admin does not exist")
	}

	// Output:
	// User exists
	// Admin does not exist
}

// ExampleCache_Set demonstrates setting values in the cache.
func ExampleCache_Set() {
	cache, err := fastcache.New[string, int](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Set some values
	if err := cache.Set("score", 100); err != nil {
		return
	}
	if err := cache.Set("level", 5); err != nil {
		return
	}

	fmt.Printf("Cache length: %d\n", cache.Len())

	// Values can be overwritten
	if err := cache.Set("score", 150); err != nil {
		return
	}
	if value, ok := cache.Get("score"); ok {
		fmt.Printf("Updated score: %d\n", value)
	}

	// Output:
	// Cache length: 2
	// Updated score: 150
}

// ExampleStats_Reset demonstrates resetting stats.
func ExampleStats_Reset() {
	var stats fastcache.Stats

	// Simulate some stats
	stats.GetCalls = 10
	stats.SetCalls = 5
	stats.Misses = 3
	stats.Hits = 7

	fmt.Printf("Before reset - GetCalls: %d, SetCalls: %d\n", stats.GetCalls, stats.SetCalls)

	// Reset the stats
	stats.Reset()

	fmt.Printf("After reset - GetCalls: %d, SetCalls: %d\n", stats.GetCalls, stats.SetCalls)

	// Output:
	// Before reset - GetCalls: 10, SetCalls: 5
	// After reset - GetCalls: 0, SetCalls: 0
}

// ExampleCache_UpdateStats demonstrates getting cache statistics.
func ExampleCache_UpdateStats() {
	cache, err := fastcache.New[string, string](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Perform some cache operations
	if err := cache.Set("key1", "value1"); err != nil {
		return
	}
	if err := cache.Set("key2", "value2"); err != nil {
		return
	}
	cache.Get("key1") // This will be a hit
	cache.Get("key3") // This will be a miss
	cache.Delete("key2")

	// Get statistics
	var stats fastcache.Stats
	cache.UpdateStats(&stats)

	fmt.Printf("Get calls: %d\n", stats.GetCalls)
	fmt.Printf("Set calls: %d\n", stats.SetCalls)
	fmt.Printf("Hits: %d\n", stats.Hits)
	fmt.Printf("Misses: %d\n", stats.Misses)
	fmt.Printf("Deletes: %d\n", stats.Deletes)
	fmt.Printf("Current entries: %d\n", stats.EntriesCount)
	fmt.Printf("Max entries: %d\n", stats.MaxEntries)

	// Output:
	// Get calls: 2
	// Set calls: 2
	// Hits: 1
	// Misses: 1
	// Deletes: 1
	// Current entries: 1
	// Max entries: 10
}
