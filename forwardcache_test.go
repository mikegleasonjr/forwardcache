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

package forwardcache

import (
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gregjones/httpcache"
)

var origin *httptest.Server
var localProxy *httptest.Server
var peerProxy *httptest.Server
var pool *Pool

func TestMain(m *testing.M) {
	setup()
	status := m.Run()
	teardown()
	os.Exit(status)
}

func TestPool(t *testing.T) {
	tests := []struct {
		origin string
		status int
		cached bool
	}{
		{origin.URL + "/jquery-3.1.1.js?buster=123", http.StatusOK, false},
		{origin.URL + "/jquery-3.1.1.js?buster=123", http.StatusOK, true},
		{origin.URL + "/no-found", http.StatusNotFound, false},
		{origin.URL + "/no-found", http.StatusNotFound, false},
	}

	for _, test := range tests {
		res, _ := pool.Client().Get(test.origin)
		res.Body.Close()

		cached := res.Header.Get("X-From-Cache") == "1"
		if cached != test.cached {
			t.Errorf("expected a different cache hit for %s: got %t, want %t", test.origin, cached, test.cached)
		}

		if res.StatusCode != test.status {
			t.Errorf("unexpected status code for %s: got %d, want %d", test.origin, res.StatusCode, test.status)
		}
	}
}

func TestPoolHeaders(t *testing.T) {
	var got string
	proxied := origin.Config.Handler
	origin.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got = req.UserAgent()
		proxied.ServeHTTP(w, req)
	})

	tests := []struct {
		ua    string
		proxy *httptest.Server
	}{
		{"ForwardCacheBot/1.0", localProxy},
		{"ForwardCacheBot/2.0", peerProxy},
	}

	for _, test := range tests {
		req, _ := http.NewRequest("GET", origin.URL+"/small.js", nil)
		req.Header.Add("User-Agent", test.ua)
		pool.Set(test.proxy.URL)
		pool.Client().Do(req)
		if got != test.ua {
			t.Errorf("invalid header sent to origin: got 'User-Agent: %s', want 'User-Agent: %s'", got, test.ua)
		}
	}

	origin.Config.Handler = proxied
	pool.Set(localProxy.URL, peerProxy.URL)
}

func ExamplePool() {
	pool := NewPool("http://10.0.1.1:3000", httpcache.NewMemoryCache())
	pool.Set("http://10.0.1.1:3000", "http://10.0.1.2:3000", "http://10.0.1.3:3000")

	// -then-

	http.DefaultTransport = pool
	http.Get("https://ajax.g[...]js/1.5.7/angular.min.js")

	// -or-

	http.DefaultClient = pool.Client()
	http.Get("https://ajax.g[...]js/1.5.7/angular.min.js")

	// -or-

	c := pool.Client()
	c.Get("https://ajax.g[...]js/1.5.7/angular.min.js")

	// ...

	http.ListenAndServe(":3000", pool.LocalProxy())
}

func setup() {
	// create an origin server and a pool with 2 members
	origin = httptest.NewServer(http.FileServer(http.Dir("./test")))
	cache := httpcache.NewMemoryCache()
	localProxy = httptest.NewServer(nil)
	peerProxy = httptest.NewServer(nil)
	pool = NewPoolOpts(localProxy.URL, cache, &PoolOptions{
		Path:     "/fwp",
		Replicas: 100,
		HashFn:   crc32.ChecksumIEEE,
	})
	pool.Set(localProxy.URL, peerProxy.URL)
	localProxy.Config.Handler = pool.LocalProxy()
	peerProxy.Config.Handler = newProxy("/fwp", cache)
}

func teardown() {
	origin.Close()
	localProxy.Close()
	peerProxy.Close()
}
