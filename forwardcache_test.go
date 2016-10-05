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
	"github.com/mikegleasonjr/forwardcache/lru"
)

var origin *httptest.Server

func TestMain(m *testing.M) {
	origin = httptest.NewServer(http.FileServer(http.Dir("./test")))
	status := m.Run()
	origin.Close()
	os.Exit(status)
}

func TestPool(t *testing.T) {
	cache := httpcache.NewMemoryCache()

	myself := httptest.NewServer(nil)
	peer := httptest.NewServer(nil)
	defer myself.Close()
	defer peer.Close()

	pool := NewPoolOpts(myself.URL, cache, &PoolOptions{
		Path:     "/fwp",
		Replicas: 100,
		HashFn:   crc32.ChecksumIEEE,
	})
	pool.Set(myself.URL, peer.URL)

	peerProxy := newProxy("/fwp", cache)
	myself.Config.Handler = pool.LocalProxy()
	peer.Config.Handler = peerProxy

	mySpy := &recorder{RoundTripper: pool.local.Transport}
	pool.local.Transport = mySpy
	peerSpy := &recorder{RoundTripper: peerProxy.Transport}
	peerProxy.Transport = peerSpy

	c := pool.Client()

	tests := []struct {
		origin string
		cached bool
	}{
		{origin.URL + "/jquery-3.1.1.js", false},
		{origin.URL + "/jquery-3.1.1.js?x=1", false},
		{origin.URL + "/jquery-3.1.1.js?x=2", false},
		{origin.URL + "/jquery-3.1.1.js?x=3", false},
		{origin.URL + "/jquery-3.1.1.js?x=4", false},
		{origin.URL + "/jquery-3.1.1.js?x=5", false},
		{origin.URL + "/small.js", false},
		{origin.URL + "/small.js?y=1", false},
		{origin.URL + "/small.js?y=2", false},
		{origin.URL + "/small.js?y=3", false},
		{origin.URL + "/small.js?y=4", false},
		{origin.URL + "/small.js?y=5", false},
		{origin.URL + "/jquery-3.1.1.js", true},
		{origin.URL + "/jquery-3.1.1.js?x=1", true},
		{origin.URL + "/jquery-3.1.1.js?x=2", true},
		{origin.URL + "/jquery-3.1.1.js?x=3", true},
		{origin.URL + "/jquery-3.1.1.js?x=4", true},
		{origin.URL + "/jquery-3.1.1.js?x=5", true},
		{origin.URL + "/small.js", true},
		{origin.URL + "/small.js?y=1", true},
		{origin.URL + "/small.js?y=2", true},
		{origin.URL + "/small.js?y=3", true},
		{origin.URL + "/small.js?y=4", true},
		{origin.URL + "/small.js?y=5", true},
		{origin.URL + "/no-found", false},
		{origin.URL + "/no-found", false},
	}

	for _, test := range tests {
		target := pool.peers.Get(test.origin)
		res, _ := c.Get(test.origin)
		res.Body.Close()

		if target == myself.URL && !mySpy.called {
			t.Errorf("unexpected proxy handling %s: got %s, want %s", test.origin, peer.URL, myself.URL)
		}

		if target == peer.URL && !peerSpy.called {
			t.Errorf("unexpected proxy handling %s: got %s, want %s", test.origin, myself.URL, peer.URL)
		}

		cached := res.Header.Get("X-From-Cache") == "1"
		if cached != test.cached {
			t.Errorf("expected a different cache hit for %s: got %t, want %t", test.origin, cached, test.cached)
		}

		mySpy.reset()
		peerSpy.reset()
	}
}

func ExamplePool() {
	cache := httpcache.Cache(httpcache.NewMemoryCache())
	cache = lru.New(cache, 32<<20)

	pool := NewPool("http://10.0.1.1", cache)
	pool.Set("http://10.0.1.1", "http://10.0.1.2", "http://10.0.1.3")

	// -then-

	http.DefaultTransport = pool
	http.Get("https://ajax.g[...]js/1.5.7/angular.min.js") // uses the pool
	http.Get("https://ajax.g[...]js/1.5.7/angular.min.js") // gets cached version (if cacheable)

	// -or-

	http.DefaultClient = pool.Client()
	http.Get("https://ajax.g[...]js/1.5.7/angular.min.js")

	// -or-

	c := pool.Client()
	c.Get("https://ajax.g[...]js/1.5.7/angular.min.js")

	// ...

	http.DefaultServeMux.Handle("/proxy", pool.LocalProxy())
}

type recorder struct {
	http.RoundTripper
	called bool
}

func (r *recorder) reset() {
	r.called = false
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.called = true
	return r.RoundTripper.RoundTrip(req)
}
