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

// Pool represents all caching proxies spread over 1 or more machines.
type Pool struct {
	local *proxy
	self  string
	opts  PoolOptions
	mu    sync.RWMutex // guards peers
	peers *consistenthash.Map
}

// PoolOptions are the configurations of a Pool. Options must be
// the same on all machines to ensure consistent hashing among peers.
type PoolOptions struct {
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

// NewPool creates a Pool and registers itself using the specified cache.
// The returned *Pool implements http.Handler and must be registered
// manually using http.Handle to serve the local proxy. See LocalProxy()
func NewPool(self string, local httpcache.Cache) *Pool {
	return &Pool{
		self:  self,
		local: newProxy(defaultPath, local),
		opts: PoolOptions{
			Path:     defaultPath,
			Replicas: defaultReplicas,
		},
	}
}

// NewPoolOpts initializes a pool of peers with the given options.
func NewPoolOpts(self string, local httpcache.Cache, opts *PoolOptions) *Pool {
	p := NewPool(self, local)
	if opts.HashFn != nil {
		p.opts.HashFn = opts.HashFn
	}
	if opts.Path != "" {
		p.opts.Path = opts.Path
	}
	if opts.Replicas != 0 {
		p.opts.Replicas = opts.Replicas
	}
	p.local.path = p.opts.Path
	return p
}

// Set updates the pool's list of peers. Each peer value should
// be a valid base URL, for example "http://example.net:8000".
// The set of peers also must contain the local peer.
func (p *Pool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.peers = consistenthash.New(p.opts.Replicas, p.opts.HashFn)
	p.peers.Add(peers...)
}

// Client returns an http.Client that uses the pool as its transport.
func (p *Pool) Client() *http.Client {
	c := new(http.Client)
	*c = *http.DefaultClient
	c.Transport = p
	return c
}

// RoundTrip makes the request go through one of the proxy. If the local
// proxy is targetted, it uses the local transport directly. Since Pool
// implements the Roundtripper interface, it can be used as a transport.
func (p *Pool) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.RLock()
	peer := p.peers.Get(req.URL.String())
	p.mu.RUnlock()

	if peer == p.self {
		return p.local.Transport.RoundTrip(req)
	}

	cpy := clone(req) // per RoundTripper contract
	query := proxyHandlerURL(peer, p.opts.Path, cpy.URL.String())
	cpy.URL = query
	cpy.Host = query.Host

	return original.RoundTrip(cpy)
}

// LocalProxy returns an http.Handler to be registered using http.Handle
// for the local proxy to serve requests for the other peers.
func (p *Pool) LocalProxy() http.Handler {
	return p.local
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
