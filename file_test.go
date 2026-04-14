package fastcache

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadSmall(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, "TestSaveLoadSmall.fastcache")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	c, err := New[string, string](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	defer c.Reset()

	key := "foobar"
	value := "abcdef"
	if err := c.Set(key, value); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	if err := c.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile error: %s", err)
	}

	c1, err := LoadFromFile[string, string](filePath)
	if err != nil {
		t.Fatalf("LoadFromFile error: %s", err)
	}
	vv, ok := c1.Get(key)
	if !ok || vv != value {
		t.Fatalf("unexpected value obtained from cache; got %q; want %q", vv, value)
	}

	// Verify that key can be overwritten.
	newValue := "234fdfd"
	if err := c1.Set(key, newValue); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	vv, ok = c1.Get(key)
	if !ok || vv != newValue {
		t.Fatalf("unexpected new value obtained from cache; got %q; want %q", vv, newValue)
	}
}

func TestLoadFileNotExist(t *testing.T) {
	c, err := LoadFromFile[string, string](`non-existing-file`)
	if err == nil {
		t.Fatalf("LoadFromFile must return error; got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadFromFile must return os.ErrNotExist; got: %s", err)
	}
	if c != nil {
		t.Fatalf("LoadFromFile must return nil cache")
	}
}

func TestSaveLoadFile(t *testing.T) {
	for _, concurrency := range []int{0, 1, 2, 4, 10} {
		t.Run(fmt.Sprintf("concurrency_%d", concurrency), func(t *testing.T) {
			testSaveLoadFile(t, concurrency)
		})
	}
}

func testSaveLoadFile(t *testing.T, concurrency int) {
	var s Stats
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, fmt.Sprintf("TestSaveLoadFile.%d.fastcache", concurrency))
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	const itemsCount = 10000
	c, err := New[string, string](itemsCount * 2)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}
	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		if err := c.Set(k, v); err != nil {
			t.Fatalf("Set error: %s", err)
		}
		vv, ok := c.Get(k)
		if !ok || v != vv {
			t.Fatalf("unexpected cache value for k=%q; got %q; want %q", k, vv, v)
		}
	}
	if concurrency == 1 {
		if err := c.SaveToFile(filePath); err != nil {
			t.Fatalf("SaveToFile error: %s", err)
		}
	} else {
		if err := c.SaveToFileConcurrent(filePath, concurrency); err != nil {
			t.Fatalf("SaveToFileConcurrent(%d) error: %s", concurrency, err)
		}
	}
	s.Reset()
	c.UpdateStats(&s)
	if s.EntriesCount != itemsCount {
		t.Fatalf("unexpected entriesCount; got %d; want %d", s.EntriesCount, itemsCount)
	}
	c.Reset()

	// Verify LoadFromFile
	c, err = LoadFromFile[string, string](filePath)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	s.Reset()
	c.UpdateStats(&s)
	if s.EntriesCount != itemsCount {
		t.Fatalf("unexpected entriesCount; got %d; want %d", s.EntriesCount, itemsCount)
	}
	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		vv, ok := c.Get(k)
		if !ok || v != vv {
			t.Fatalf("unexpected cache value for k=%q; got %q; want %q", k, vv, v)
		}
	}
	c.Reset()

	// Verify LoadFromFileOrNew
	c, err = LoadFromFileOrNew[string, string](filePath, itemsCount*2)
	if err != nil {
		t.Fatalf("LoadFromFileOrNew error: %s", err)
	}
	s.Reset()
	c.UpdateStats(&s)
	if s.EntriesCount != itemsCount {
		t.Fatalf("unexpected entriesCount; got %d; want %d", s.EntriesCount, itemsCount)
	}
	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		vv, ok := c.Get(k)
		if !ok || v != vv {
			t.Fatalf("unexpected cache value for k=%q; got %q; want %q", k, vv, v)
		}
	}
	c.Reset()
}

func TestSaveLoadStruct(t *testing.T) {
	type User struct {
		ID   int
		Name string
		Tags []string
	}

	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(tmpDir, "TestSaveLoadStruct.fastcache")
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	c, err := New[int, User](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	users := []User{
		{ID: 1, Name: "Alice", Tags: []string{"admin", "user"}},
		{ID: 2, Name: "Bob", Tags: []string{"user"}},
		{ID: 3, Name: "Charlie", Tags: nil},
	}

	for _, u := range users {
		if err := c.Set(u.ID, u); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	if err := c.SaveToFile(filePath); err != nil {
		t.Fatalf("SaveToFile error: %s", err)
	}

	c2, err := LoadFromFile[int, User](filePath)
	if err != nil {
		t.Fatalf("LoadFromFile error: %s", err)
	}

	for _, u := range users {
		got, ok := c2.Get(u.ID)
		if !ok {
			t.Fatalf("user %d not found after load", u.ID)
		}
		if got.ID != u.ID || got.Name != u.Name {
			t.Fatalf("unexpected user; got %+v; want %+v", got, u)
		}
		if len(got.Tags) != len(u.Tags) {
			t.Fatalf("unexpected tags length; got %d; want %d", len(got.Tags), len(u.Tags))
		}
	}
}

func TestLoadFromFileOrNew_NonExistent(t *testing.T) {
	c, err := LoadFromFileOrNew[string, string]("non-existing-file", 100)
	if err != nil {
		t.Fatalf("LoadFromFileOrNew error: %s", err)
	}
	if c == nil {
		t.Fatal("LoadFromFileOrNew returned nil")
	}

	// Should be an empty cache
	if c.Len() != 0 {
		t.Fatalf("expected empty cache; got %d entries", c.Len())
	}

	// Should be usable
	if err := c.Set("key", "value"); err != nil {
		t.Fatalf("Set error: %s", err)
	}
	if v, ok := c.Get("key"); !ok || v != "value" {
		t.Fatalf("unexpected value; got %q; want %q", v, "value")
	}
}

func TestSaveToLoadFrom(t *testing.T) {
	const itemsCount = 1000
	c, err := New[string, string](itemsCount * 2)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		if err := c.Set(k, v); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	var buf bytes.Buffer
	if err := c.SaveTo(&buf); err != nil {
		t.Fatalf("SaveTo error: %s", err)
	}

	c2, err := LoadFrom[string, string](&buf)
	if err != nil {
		t.Fatalf("LoadFrom error: %s", err)
	}

	if c2.Len() != itemsCount {
		t.Fatalf("unexpected length; got %d; want %d", c2.Len(), itemsCount)
	}

	for i := range itemsCount {
		k := fmt.Sprintf("key %d", i)
		v := fmt.Sprintf("value %d", i)
		vv, ok := c2.Get(k)
		if !ok || v != vv {
			t.Fatalf("unexpected cache value for k=%q; got %q; want %q", k, vv, v)
		}
	}
}

func TestSaveToLoadFrom_Struct(t *testing.T) {
	type User struct {
		ID   int
		Name string
		Tags []string
	}

	c, err := New[int, User](100)
	if err != nil {
		t.Fatalf("New error: %s", err)
	}

	users := []User{
		{ID: 1, Name: "Alice", Tags: []string{"admin", "user"}},
		{ID: 2, Name: "Bob", Tags: []string{"user"}},
		{ID: 3, Name: "Charlie", Tags: nil},
	}

	for _, u := range users {
		if err := c.Set(u.ID, u); err != nil {
			t.Fatalf("Set error: %s", err)
		}
	}

	var buf bytes.Buffer
	if err := c.SaveTo(&buf); err != nil {
		t.Fatalf("SaveTo error: %s", err)
	}

	c2, err := LoadFrom[int, User](&buf)
	if err != nil {
		t.Fatalf("LoadFrom error: %s", err)
	}

	for _, u := range users {
		got, ok := c2.Get(u.ID)
		if !ok {
			t.Fatalf("user %d not found after load", u.ID)
		}
		if got.ID != u.ID || got.Name != u.Name {
			t.Fatalf("unexpected user; got %+v; want %+v", got, u)
		}
		if len(got.Tags) != len(u.Tags) {
			t.Fatalf("unexpected tags length; got %d; want %d", len(got.Tags), len(u.Tags))
		}
	}
}

func TestLoadFrom_EmptyReader(t *testing.T) {
	var buf bytes.Buffer
	_, err := LoadFrom[string, string](&buf)
	if err == nil {
		t.Fatal("LoadFrom must return error for empty reader")
	}
}
