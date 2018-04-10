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
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gregjones/httpcache"
)

func TestProxy(t *testing.T) {
	okRoundTrip := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		res := okResponse()
		res.Header.Set("X-Url", req.URL.String())
		return res, nil
	})

	origin := newRoundTripperMock().
		add("GET", "http://cdn.com/jquery.js", okRoundTrip).
		add("POST", "http://cdn.com/jquery.js", okRoundTrip)

	proxy := newProxy("/p", httpcache.NewMemoryCache(), origin, DefaultBufferPool)

	testCases := []struct {
		method     string
		path       string
		status     int
		body       string
		xFromCache string
		xURL       string
	}{
		{"GET", "/another", http.StatusBadGateway, "", "", ""},
		{"GET", "/p?q=", http.StatusBadGateway, "", "", ""},
		{"GET", "/p?q=" + url.QueryEscape("http://10.0.1.%31/"), http.StatusBadGateway, "", "", ""},
		{"GET", "/p?q=" + url.QueryEscape("http://cdn.com/jquery.js"), http.StatusOK, "OK", "", "http://cdn.com/jquery.js"},
		{"GET", "/p?q=" + url.QueryEscape("http://cdn.com/jquery.js"), http.StatusOK, "OK", "1", "http://cdn.com/jquery.js"},
		{"POST", "/p?q=" + url.QueryEscape("http://cdn.com/jquery.js"), http.StatusOK, "OK", "", "http://cdn.com/jquery.js"},
		{"POST", "/p?q=" + url.QueryEscape("http://cdn.com/jquery.js"), http.StatusOK, "OK", "", "http://cdn.com/jquery.js"},
		{"GET", "/p?q=" + url.QueryEscape("http://cdn.com/bootstrap.js"), http.StatusNotFound, "Not Found", "", ""},
		{"GET", "/p?q=" + url.QueryEscape("http://cdn.com/bootstrap.js"), http.StatusNotFound, "Not Found", "", ""},
	}
	for _, tC := range testCases {
		path, _ := url.QueryUnescape(tC.path)
		t.Run(tC.method+path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req, _ := http.NewRequest(tC.method, tC.path, nil)
			proxy.ServeHTTP(rr, req)

			if rr.Code != tC.status {
				t.Errorf("proxy sent wrong status: got %d, want %d", rr.Code, tC.status)
			}

			if body := rr.Body.String(); body != tC.body {
				t.Errorf("proxy returned unexpected body: got %s want %s", body, tC.body)
			}

			if xFromCache := rr.HeaderMap.Get(httpcache.XFromCache); xFromCache != tC.xFromCache {
				t.Errorf("unexpected %q header: got %q, want %q", httpcache.XFromCache, xFromCache, tC.xFromCache)
			}

			if xURL := rr.HeaderMap.Get("X-Url"); xURL != tC.xURL {
				t.Errorf("unexpected X-Url header: got %q, want %q", xURL, tC.xURL)
			}
		})
	}
}

func BenchmarkProxy(b *testing.B) {
	body := strings.NewReader("OK")
	res := okResponse()
	origin := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body.Seek(0, io.SeekStart)
		res.Body = ioutil.NopCloser(body)
		res.Request = req
		return res, nil
	})

	handler := newProxy("/proxy", &noopCache{}, origin, DefaultBufferPool)
	discard := &discarder{}
	req, _ := http.NewRequest("GET", "/proxy?q="+url.QueryEscape("http://cdn.com/jquery.js"), nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(discard, req)
	}
}

type discarder struct{}

func (*discarder) Header() http.Header         { return make(http.Header) }
func (*discarder) Write(b []byte) (int, error) { return len(b), nil }
func (*discarder) WriteHeader(int)             {}

type noopCache struct{}

func (c *noopCache) Set(key string, resp []byte)   {}
func (c *noopCache) Delete(key string)             {}
func (c *noopCache) Get(key string) ([]byte, bool) { return nil, false }

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (rt roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return rt(req) }

type roundTripperMock struct {
	mocks map[string]roundTripperFunc
}

func newRoundTripperMock() *roundTripperMock {
	return &roundTripperMock{map[string]roundTripperFunc{}}
}

func (m *roundTripperMock) add(method, url string, f roundTripperFunc) *roundTripperMock {
	m.mocks[method+" "+url] = f
	return m
}

func (m *roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	if f, ok := m.mocks[req.Method+" "+req.URL.String()]; ok {
		return f(req)
	}

	return &http.Response{
		Status:        "404 Not Found",
		StatusCode:    http.StatusNotFound,
		Header:        make(http.Header),
		Body:          ioutil.NopCloser(strings.NewReader("Not Found")),
		ContentLength: 9,
		Request:       req,
	}, nil
}

func okResponse() *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          ioutil.NopCloser(strings.NewReader("OK")),
		ContentLength: 2,
		Header: map[string][]string{
			"date":    []string{time.Now().Format(time.RFC1123)},
			"Expires": []string{time.Now().Add(time.Hour).Format(time.RFC1123)},
		},
	}
}
