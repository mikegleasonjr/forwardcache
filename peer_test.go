/*
Copyright 2018 Mike Gleason jr Couturier.

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
	"bytes"
	"net/http"
	"testing"
)

func TestPeer(t *testing.T) {
	hash := newHashMock().
		with("http://self.com:3000", 0).
		with("http://peer.com:3000", 1).
		with("http://some.url/res-self.js", 0).
		with("http://some.url/res-peer.js", 1)

	peerTransport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		res := okResponse()
		res.Header.Set("X-Source", "self")
		return res, nil
	})

	clientTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		res := okResponse()
		res.Header.Set("X-Source", "client")
		return res, nil
	})

	peer := NewPeer("http://self.com:3000",
		WithPeerTransport(peerTransport),
		WithClient(NewClient(
			WithPool("http://self.com:3000", "http://peer.com:3000"),
			WithHashFn(hash.fn),
			WithClientTransport(clientTransport),
		)),
	)

	testCases := []struct {
		url  string
		want string
	}{
		{"http://some.url/res-self.js", "self"},
		{"http://some.url/res-peer.js", "client"},
	}
	for _, tC := range testCases {
		t.Run(tC.url, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tC.url, nil)
			res, err := peer.RoundTrip(req)
			// io.Copy(ioutil.Discard, res.Body)
			// res.Body.Close()

			if err != nil {
				t.Fatalf("unexpected error: got %q, want <nil>", err)
			}

			if got := res.Header.Get("X-Source"); got != tC.want {
				t.Fatalf("unexpected transport handler: got %q, want %q", got, tC.want)
			}
		})
	}
}

func ExampleNewPeer() {
	peer := NewPeer("http://10.0.1.1:3000")
	peer.SetPool("http://10.0.1.1:3000", "http://10.0.1.2:3000")

	// -or-

	client := NewClient(WithPool("http://10.0.1.1:3000", "http://10.0.1.2:3000"))
	peer = NewPeer("http://10.0.1.1:3000", WithClient(client))

	// -then-

	http.DefaultTransport = peer
	http.Get("https://...js/1.5.7/angular.min.js")

	// -or-

	http.DefaultClient = peer.HTTPClient()
	http.Get("https://...js/1.5.7/angular.min.js")

	// -or-

	c := peer.HTTPClient()
	c.Get("https://...js/1.5.7/angular.min.js")

	// ...

	http.ListenAndServe(":3000", peer.Handler())
}

type hashMock struct {
	mocks  map[string]uint32
	hashTo uint32
}

func newHashMock() *hashMock { return &hashMock{mocks: map[string]uint32{}} }

func (h *hashMock) with(url string, hashTo uint32) *hashMock {
	h.mocks[url] = hashTo
	return h
}

func (h *hashMock) fn(data []byte) uint32 {
	for k, v := range h.mocks {
		if bytes.Contains(data, []byte(k)) {
			return v
		}
	}
	return h.hashTo
}
