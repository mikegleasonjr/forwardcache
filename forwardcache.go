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

// Package forwardcache provides a forward caching proxy that works
// across a set of peer processes.
//
// When an http request is made, a peer is chosen to proxy the request
// according to the requested url's canonical owner.
//
// If the content is cacheable as per the HTTP RFC, it will get cached
// on the proxy and the response is then returned to the client. (thanks
// to github.com/gregjones/httpcache)
//
// Note that the peers are not real HTTP proxies. They are themselves
// querying the origin servers and copying the response back to clients.
// It has the benefit of being able to cache TLS requests.
//
// The result is that it is only suitable for use as a distributed 'private'
// cache since the requests are fully intercepted by the peers.
package forwardcache

import (
	"hash/crc32"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/gregjones/httpcache"
	"github.com/mikegleasonjr/forwardcache/consistenthash"
)

const (
	defaultPath     = "/proxy"
	defaultReplicas = 50
)

// Pool represents all caching proxies spread over 1 or more machines. It
// also acts as a participating peer.
type Pool struct {
	*Client
	self      string
	local     *proxy
	transport http.RoundTripper
	buffers   httputil.BufferPool
}

// NewPool creates a Pool and registers itself using the specified cache.
// The returned *Pool implements http.Handler and must be registered
// manually using http.Handle to serve the local proxy. See LocalProxy()
func NewPool(self string, local httpcache.Cache, options ...func(*Pool)) *Pool {
	p := &Pool{self: self}
	for _, option := range options {
		option(p)
	}
	if p.transport == nil {
		p.transport = http.DefaultTransport
	}
	if p.Client == nil {
		p.Client = NewClient()
	}
	p.local = newProxy(p.path, local, p.transport, p.buffers)
	return p
}

// LocalProxy returns an http.Handler to be registered using http.Handle
// for the local proxy to serve requests.
func (p *Pool) LocalProxy() http.Handler {
	return p.local
}

// WithProxyTransport lets you configure a custom
// transport used between the local proxy and the origins.
// Defaults to http.DefaultTransport.
func WithProxyTransport(t http.RoundTripper) func(*Pool) {
	return func(p *Pool) {
		p.transport = t
	}
}

// WithClient lets you configure a custom pool client.
// Defaults to NewClient().
func WithClient(c *Client) func(*Pool) {
	return func(p *Pool) {
		p.Client = c
	}
}

// WithBufferPool lets you configure a custom buffer pool.
// Defaults to not using a buffer pool.
func WithBufferPool(b httputil.BufferPool) func(*Pool) {
	return func(p *Pool) {
		p.buffers = b
	}
}

// WithDefaultBufferPool lets you use the default 32k buffer pool.
// Defaults to not using a buffer pool.
func WithDefaultBufferPool(b httputil.BufferPool) func(*Pool) {
	return func(p *Pool) {
		p.buffers = DefaultBufferPool
	}
}

// Client represents a nonparticipating client in the pool. It can
// issue requests to the pool but not proxy requests for others.
type Client struct {
	path      string
	replicas  int
	hashFn    consistenthash.Hash
	transport http.RoundTripper
	mu        sync.RWMutex // guards peers
	peers     *consistenthash.Map
}

// NewClient creates a Client.
func NewClient(options ...func(*Client)) *Client {
	c := &Client{
		path:      defaultPath,
		replicas:  defaultReplicas,
		hashFn:    crc32.ChecksumIEEE,
		transport: http.DefaultTransport,
	}
	for _, option := range options {
		option(c)
	}
	return c
}

// Set updates the pool's list of peers. Each peer value should
// be a valid base URL, for example "http://example.net:8000".
func (c *Client) Set(peers ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.peers = consistenthash.New(c.replicas, c.hashFn)
	c.peers.Add(peers...)
}

// HTTPClient returns an http.Client that uses the pool as its transport.
func (c *Client) HTTPClient() *http.Client {
	cl := new(http.Client)
	*cl = *http.DefaultClient
	cl.Transport = c
	return cl
}

// RoundTrip makes the request go through one of the proxy. If the local
// proxy is targetted, it uses the local transport directly. Since Client
// implements the Roundtripper interface, it can be used as a transport.
func (c *Client) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.RLock()
	peer := c.peers.Get(req.URL.String())
	c.mu.RUnlock()

	cpy := clone(req) // per RoundTripper contract
	query := proxyHandlerURL(peer, c.path, cpy.URL.String())
	cpy.URL = query
	cpy.Host = query.Host

	return c.transport.RoundTrip(cpy)
}

// WithPath specifies the HTTP path that will serve proxy requests.
// Defaults to "/proxy".
func WithPath(p string) func(*Client) {
	return func(c *Client) {
		c.path = p
	}
}

// WithReplicas specifies the number of key replicas on the consistent hash.
// Defaults to 50.
func WithReplicas(r int) func(*Client) {
	return func(c *Client) {
		c.replicas = r
	}
}

// WithHashFn specifies the hash function of the consistent hash.
// Defaults to crc32.ChecksumIEEE.
func WithHashFn(h consistenthash.Hash) func(*Client) {
	return func(c *Client) {
		c.hashFn = h
	}
}

// WithClientTransport lets you configure a custom transport
// used between the local client and the proxies.
// Defaults to http.DefaultTransport.
func WithClientTransport(t http.RoundTripper) func(*Client) {
	return func(c *Client) {
		c.transport = t
	}
}

// builds the url that handles proxy requests on the selected peer
func proxyHandlerURL(peer, path, origin string) *url.URL {
	u, _ := url.Parse(peer)
	u.Path = path
	u.RawQuery = "q=" + url.QueryEscape(origin)
	return u
}

// clones a request, credits goes to:
// https://github.com/golang/oauth2/blob/master/transport.go#L36
func clone(r *http.Request) *http.Request {
	r2 := new(http.Request)
	*r2 = *r
	r2.Header = make(http.Header, len(r.Header))
	for k, s := range r.Header {
		r2.Header[k] = append([]string(nil), s...)
	}
	return r2
}
