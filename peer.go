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
// across a set of peer processes. In simple terms, it is a distributed
// cache for HEAD and GET requests. It follows the HTTP RFC so it will
// only cache cacheable responses (like browsers do).
//
// When an http request is made, a peer is chosen to handle the request
// according to the requested url's canonical owner.
//
// If the content is cacheable as per the HTTP RFC, it will get cached
// on the peer and the response is then returned to the client. (thanks
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
	"net/http/httputil"

	"github.com/gregjones/httpcache"
)

// Peer is a peer in the pool. It handles and cache the requests for the clients.
// It is also able to issue requests on its own to other peers when a resource
// belongs to it.
type Peer struct {
	*Client
	handler   *proxy
	self      string
	cache     httpcache.Cache
	transport http.RoundTripper
	buffers   httputil.BufferPool
}

// NewPeer creates a Peer.
// The returned *Peer implements http.Handler and must be registered
// manually using http.Handle to serve local requests. See Handler().
func NewPeer(self string, options ...func(*Peer)) *Peer {
	p := &Peer{
		Client:    NewClient(),
		self:      self,
		transport: http.DefaultTransport,
		cache:     httpcache.NewMemoryCache(),
	}

	for _, option := range options {
		option(p)
	}

	p.handler = newProxy(p.Client.path, p.cache, p.transport, p.buffers)
	return p
}

// Handler returns an http.Handler to be registered using http.Handle
// for the local Peer to serve requests.
func (p *Peer) Handler() http.Handler {
	return p.handler
}

// RoundTrip makes the request go through one of the peer using its internal
// Client. If the local peer is targeted, it uses the local handler directly.
// Since Peer implements the Roundtripper interface, it can be used as a transport.
func (p *Peer) RoundTrip(req *http.Request) (*http.Response, error) {
	peer := p.Client.choosePeer(req.URL.String())

	if peer == p.self {
		return p.handler.Transport.RoundTrip(req)
	}

	return p.Client.roundTripTo(peer, req)
}

// WithClient lets you configure a custom pool client.
// Defaults to NewClient(). If a Client is not specified
// upon Peer creation, SetPool(...) must be called to set
// the list of peers.
func WithClient(c *Client) func(*Peer) {
	return func(p *Peer) {
		p.Client = c
	}
}

// WithPeerTransport lets you configure a custom
// transport used between the local peer and origins.
// Defaults to http.DefaultTransport.
func WithPeerTransport(t http.RoundTripper) func(*Peer) {
	return func(p *Peer) {
		p.transport = t
	}
}

// WithBufferPool lets you configure a custom buffer pool.
// Defaults to not using a buffer pool.
func WithBufferPool(b httputil.BufferPool) func(*Peer) {
	return func(p *Peer) {
		p.buffers = b
	}
}

// WithDefaultBufferPool lets you use the default 32k buffer pool.
// Defaults to not using a buffer pool.
func WithDefaultBufferPool(b httputil.BufferPool) func(*Peer) {
	return func(p *Peer) {
		p.buffers = DefaultBufferPool
	}
}

// WithCache lets you use a custom httpcache.Cache.
// Defaults to httpcache.MemoryCache.
func WithCache(c httpcache.Cache) func(*Peer) {
	return func(p *Peer) {
		p.cache = c
	}
}
