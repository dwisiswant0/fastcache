package fastcache_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"go.dw1.io/fastcache"
)

// ExampleCache_SaveToFile demonstrates saving cache data to a file.
func ExampleCache_SaveToFile() {
	// Create a temporary file for the example
	tmpDir, _ := os.MkdirTemp("", "fastcache-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()
	filePath := filepath.Join(tmpDir, "cache.dat")

	cache, err := fastcache.New[string, int](100)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Add some data
	if err := cache.Set("users", 1000); err != nil {
		return
	}
	if err := cache.Set("posts", 5000); err != nil {
		return
	}

	// Save to file
	if err := cache.SaveToFile(filePath); err != nil {
		fmt.Println("Error saving:", err)
		return
	}

	fmt.Println("Cache saved successfully")

	// Output:
	// Cache saved successfully
}

// ExampleLoadFromFile demonstrates loading cache data from a file.
func ExampleLoadFromFile() {
	// Create a temporary file for the example
	tmpDir, _ := os.MkdirTemp("", "fastcache-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()
	filePath := filepath.Join(tmpDir, "cache.dat")

	// First, create and save a cache
	cache, err := fastcache.New[string, string](100)
	if err != nil {
		return
	}
	if err := cache.Set("greeting", "hello"); err != nil {
		return
	}
	if err := cache.Set("language", "Go"); err != nil {
		return
	}
	if err := cache.SaveToFile(filePath); err != nil {
		fmt.Println("Error saving:", err)
		return
	}
	cache.Reset()

	// Now load it back
	loadedCache, err := fastcache.LoadFromFile[string, string](filePath)
	if err != nil {
		fmt.Println("Error loading:", err)
		return
	}
	defer loadedCache.Reset()

	// Access the loaded data
	if value, ok := loadedCache.Get("greeting"); ok {
		fmt.Println("Greeting:", value)
	}
	if value, ok := loadedCache.Get("language"); ok {
		fmt.Println("Language:", value)
	}

	// Output:
	// Greeting: hello
	// Language: Go
}

// ExampleLoadFromFileOrNew demonstrates loading from file with fallback.
func ExampleLoadFromFileOrNew() {
	tmpDir, _ := os.MkdirTemp("", "fastcache-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()
	filePath := filepath.Join(tmpDir, "cache.dat")

	// File doesn't exist, so it creates a new cache
	cache, err := fastcache.LoadFromFileOrNew[string, int](filePath, 50)
	if err != nil {
		return
	}
	defer cache.Reset()

	fmt.Printf("New cache created with capacity: %d\n", 50)
	fmt.Println("Cache length:", cache.Len())

	// Output:
	// New cache created with capacity: 50
	// Cache length: 0
}

// ExampleCache_SaveTo demonstrates saving to a writer.
func ExampleCache_SaveTo() {
	cache, err := fastcache.New[string, string](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	if err := cache.Set("example", "data"); err != nil {
		return
	}

	// Save to a buffer (could be any io.Writer)
	var buf bytes.Buffer
	if err := cache.SaveTo(&buf); err != nil {
		fmt.Println("Error saving:", err)
		return
	}

	fmt.Println("Successfully saved data to buffer")

	// Output:
	// Successfully saved data to buffer
}

// ExampleLoadFrom demonstrates loading from a reader.
func ExampleLoadFrom() {
	// First create some data in a buffer
	cache, err := fastcache.New[string, int](10)
	if err != nil {
		return
	}
	if err := cache.Set("count", 42); err != nil {
		return
	}

	var buf bytes.Buffer
	if err := cache.SaveTo(&buf); err != nil {
		fmt.Println("Error saving:", err)
		return
	}
	cache.Reset()

	// Now load from the buffer
	loadedCache, err := fastcache.LoadFrom[string, int](&buf)
	if err != nil {
		fmt.Println("Error loading:", err)
		return
	}
	defer loadedCache.Reset()

	if value, ok := loadedCache.Get("count"); ok {
		fmt.Println("Loaded count:", value)
	}

	// Output:
	// Loaded count: 42
}

// ExampleCache_SaveToFileConcurrent demonstrates concurrent saving.
func ExampleCache_SaveToFileConcurrent() {
	tmpDir, _ := os.MkdirTemp("", "fastcache-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()
	filePath := filepath.Join(tmpDir, "cache.dat")

	cache, err := fastcache.New[int, string](1000)
	if err != nil {
		return
	}
	defer cache.Reset()

	// Add many entries
	for i := range 100 {
		if err := cache.Set(i, fmt.Sprintf("value-%d", i)); err != nil {
			return
		}
	}

	// Save using 4 concurrent workers
	if err := cache.SaveToFileConcurrent(filePath, 4); err != nil {
		fmt.Println("Error saving:", err)
		return
	}

	fmt.Println("Cache saved with concurrency")

	// Output:
	// Cache saved with concurrency
}

// ExampleSaveToFunc demonstrates saving some entries, converted to a type
// that gob can encode.
func ExampleSaveToFunc() {
	// gob does not encode unexported fields, so SaveTo rejects session.
	type session struct {
		user  string
		token string
	}

	cache, err := fastcache.New[string, session](10)
	if err != nil {
		return
	}
	defer cache.Reset()

	if err := cache.Set("s1", session{user: "alice", token: "t1"}); err != nil {
		return
	}
	if err := cache.Set("s2", session{token: "t2"}); err != nil {
		return
	}
	if err := cache.Set("s3", session{user: "bob", token: "t3"}); err != nil {
		return
	}

	// Save the user of each signed-in session, and never the token.
	var buf bytes.Buffer

	err = fastcache.SaveToFunc(cache, &buf, func(_ string, s session) (string, bool) {
		return s.user, s.user != ""
	})
	if err != nil {
		fmt.Println("Error saving:", err)
		return
	}

	loaded, err := fastcache.LoadFrom[string, string](&buf)
	if err != nil {
		fmt.Println("Error loading:", err)
		return
	}
	defer loaded.Reset()

	for key, user := range loaded.AllFIFO() {
		fmt.Println(key, user)
	}

	// Output:
	// s1 alice
	// s3 bob
}

// ExampleLoadFromFunc demonstrates loading saved entries into an existing
// cache with a smaller capacity.
func ExampleLoadFromFunc() {
	saved, err := fastcache.New[string, int](10)
	if err != nil {
		return
	}

	for i, key := range []string{"a", "b", "c", "d"} {
		if err := saved.Set(key, i); err != nil {
			return
		}
	}

	var buf bytes.Buffer
	if err := saved.SaveTo(&buf); err != nil {
		fmt.Println("Error saving:", err)
		return
	}

	saved.Reset()

	// Entries load oldest first, so the cache keeps the newest that fit.
	cache, err := fastcache.New[string, int](2)
	if err != nil {
		return
	}
	defer cache.Reset()

	if err := fastcache.LoadFromFunc(&buf, cache.Set); err != nil {
		fmt.Println("Error loading:", err)
		return
	}

	for key, value := range cache.AllFIFO() {
		fmt.Printf("%s:%d\n", key, value)
	}

	// Output:
	// c:2
	// d:3
}
