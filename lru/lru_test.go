/*
Copyright 2016 Mike Gleason jr Couturier.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package lru

import (
	"bytes"
	"crypto/rand"
	"strconv"
	"sync"
	"testing"

	"github.com/gregjones/httpcache"
)

func TestSet(t *testing.T) {
	cache := httpcache.NewMemoryCache()
	lru := New(cache, 10)
	tests := []struct {
		key     string
		val     []byte
		present []string
		absent  []string
	}{
		{"key1", randBytes(4), []string{"key1"}, []string{}},                           // cap: 6
		{"key2", randBytes(4), []string{"key2", "key1"}, []string{}},                   // cap: 2
		{"key3", randBytes(4), []string{"key3", "key2"}, []string{"key1"}},             // cap: 2
		{"key4", randBytes(6), []string{"key4", "key3"}, []string{"key2"}},             // cap: 0
		{"key5", randBytes(12), []string{"key5"}, []string{"key4", "key3"}},            // cap: -2
		{"key6", randBytes(1), []string{"key6"}, []string{"key5"}},                     // cap: 9
		{"key7", randBytes(1), []string{"key7", "key6"}, []string{}},                   // cap: 8
		{"key8", randBytes(8), []string{"key8", "key7", "key6"}, []string{}},           // cap: 0
		{"key7", randBytes(1), []string{"key7", "key8", "key6"}, []string{}},           // cap: 0
		{"key9", randBytes(1), []string{"key9", "key7", "key8"}, []string{"key6"}},     // cap: 0
		{"key8", randBytes(9), []string{"key8", "key9"}, []string{"key7"}},             // cap: 0
		{"key10", randBytes(1), []string{"key10", "key8"}, []string{"key9"}},           // cap: 0
		{"key8", randBytes(6), []string{"key8", "key10"}, []string{}},                  // cap: 3
		{"key11", randBytes(3), []string{"key11", "key8", "key10"}, []string{}},        // cap: 0
		{"key12", randBytes(5), []string{"key12", "key11"}, []string{"key8", "key10"}}, // cap: 2
	}

	for _, test := range tests {
		lru.Set(test.key, test.val)

		for _, key := range test.present {
			if val, exists := cache.Get(key); !exists {
				t.Errorf("expected '%s' to be in the cache after inserting '%s'", key, test.key)
			} else if test.key == key && bytes.Compare(test.val, val) != 0 {
				t.Errorf("value mismatch for '%s': got '%v', want '%v'", key, val, test.val)
			}
		}

		for _, key := range test.absent {
			if _, exists := cache.Get(key); exists {
				t.Errorf("unexpected item in cache '%s' after inserting '%s'", key, test.key)
			}
		}
	}
}

func TestGet(t *testing.T) {
	cache := httpcache.NewMemoryCache()
	lru := New(cache, 10)

	if _, exists := lru.Get("unknown"); exists {
		t.Errorf("unexpected key '%s' in cache", "unknown")
	}

	key1val := randBytes(5)
	lru.Set("key1", key1val)      // key1
	lru.Set("key2", randBytes(5)) // key2, key1

	val, exists := lru.Get("key1") // key1, key2
	if !exists {
		t.Errorf("expected key '%s' to be found in cache", "key1")
	}

	if bytes.Compare(key1val, val) != 0 {
		t.Errorf("bad value for '%s': got '%s', want '%s'", "key1", val, key1val)
	}

	lru.Set("key3", randBytes(5))

	if _, exists := lru.Get("key2"); exists {
		t.Errorf("unexpected key '%s' in cache", "key2")
	}

	if _, exists := lru.Get("key1"); !exists {
		t.Errorf("expected key '%s' to be found in cache", "key1")
	}
}

func TestDelete(t *testing.T) {
	cache := httpcache.NewMemoryCache()
	lru := New(cache, 10)

	lru.Set("key1", randBytes(4))
	lru.Delete("key1")
	if _, exists := lru.Get("key1"); exists {
		t.Errorf("unexpected key '%s' in cache", "key1")
	}
}

func TestRace(t *testing.T) {
	var wg sync.WaitGroup
	cache := httpcache.NewMemoryCache()
	lru := New(cache, 1024)
	worker := func(key string, val []byte) {
		for i := 0; i < 10000; i++ {
			lru.Set(key, val)
			if i%2 == 0 {
				lru.Get(key)
			}
			if i%3 == 0 {
				lru.Delete(key)
			}
		}
		wg.Done()
	}

	for i := 0; i < 8; i++ {
		wg.Add(2)
		go worker("key"+strconv.Itoa(i), randBytes(10))
		go worker("key"+strconv.Itoa(i), randBytes(15))
	}
	wg.Wait()
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return b
}
