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
	"net/http"
	"net/url"
	"sync"

	"github.com/gregjones/httpcache"
	"github.com/mikegleasonjr/forwardcache/consistenthash"
)

const (
	defaultPath     = "/proxy"
	defaultReplicas = 50
)

var original http.RoundTripper

func init() {
	original = http.DefaultTransport
}

// Pool represents all caching proxies spread over 1 or more machines. It
// also acts as a participating peer.
type Pool struct {
	*PoolClient
	self  string
	local *proxy
}

// NewPool creates a Pool and registers itself using the specified cache.
// The returned *Pool implements http.Handler and must be registered
// manually using http.Handle to serve the local proxy. See LocalProxy()
func NewPool(self string, local httpcache.Cache) *Pool {
	p := &Pool{
		self:       self,
		local:      newProxy(defaultPath, local),
		PoolClient: NewClient(),
	}
	return p
}

// NewPoolOpts initializes a pool of peers with the given options.
func NewPoolOpts(self string, local httpcache.Cache, opts *ClientOptions) *Pool {
	p := &Pool{
		self:       self,
		local:      newProxy(defaultPath, local),
		PoolClient: NewClientOpts(opts),
	}
	if opts.Path != "" {
		p.local.path = opts.Path
	}
	return p
}

// LocalProxy returns an http.Handler to be registered using http.Handle
// for the local proxy to serve requests.
func (p *Pool) LocalProxy() http.Handler {
	return p.local
}

// PoolClient represents a nonparticipating client in the pool. It can
// issue requests to the pool but not proxy requests for others.
type PoolClient struct {
	opts  ClientOptions
	mu    sync.RWMutex // guards peers
	peers *consistenthash.Map
}

// ClientOptions are the configurations of a PoolClient. Options must be
// the same on all machines to ensure consistent hashing among peers.
type ClientOptions struct {
	// Path specifies the HTTP path that will serve proxy requests.
	// If blank, it defaults to "/proxy".
	Path string

	// Replicas specifies the number of key replicas on the consistent hash.
	// If blank, it defaults to 50.
	Replicas int

	// HashFn specifies the hash function of the consistent hash.
	// If blank, it defaults to crc32.ChecksumIEEE.
	HashFn consistenthash.Hash
}

// NewClient creates a PoolClient.
func NewClient(peers ...string) *PoolClient {
	c := &PoolClient{
		opts: ClientOptions{
			Path:     defaultPath,
			Replicas: defaultReplicas,
		},
	}
	if len(peers) > 0 {
		c.Set(peers...)
	}
	return c
}

// NewClientOpts initializes a PoolClient with the given options.
func NewClientOpts(opts *ClientOptions, peers ...string) *PoolClient {
	c := NewClient(peers...)
	if opts.HashFn != nil {
		c.opts.HashFn = opts.HashFn
	}
	if opts.Path != "" {
		c.opts.Path = opts.Path
	}
	if opts.Replicas != 0 {
		c.opts.Replicas = opts.Replicas
	}
	return c
}

// Set updates the pool's list of peers. Each peer value should
// be a valid base URL, for example "http://example.net:8000".
func (c *PoolClient) Set(peers ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.peers = consistenthash.New(c.opts.Replicas, c.opts.HashFn)
	c.peers.Add(peers...)
}

// Client returns an http.Client that uses the pool as its transport.
func (c *PoolClient) Client() *http.Client {
	cl := new(http.Client)
	*cl = *http.DefaultClient
	cl.Transport = c
	return cl
}

// RoundTrip makes the request go through one of the proxy. If the local
// proxy is targetted, it uses the local transport directly. Since PoolClient
// implements the Roundtripper interface, it can be used as a transport.
func (c *PoolClient) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.RLock()
	peer := c.peers.Get(req.URL.String())
	c.mu.RUnlock()

	cpy := clone(req) // per RoundTripper contract
	query := proxyHandlerURL(peer, c.opts.Path, cpy.URL.String())
	cpy.URL = query
	cpy.Host = query.Host

	return original.RoundTrip(cpy)
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
