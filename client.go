package forwardcache

import (
	"hash/crc32"
	"net/http"
	"net/url"
	"sync"

	"github.com/mikegleasonjr/forwardcache/consistenthash"
)

const (
	defaultPath     = "/proxy"
	defaultReplicas = 50
)

// Client represents a nonparticipating client in the pool. It delegates
// requests to the responsible peer.
type Client struct {
	path      string
	replicas  int
	hashFn    consistenthash.Hash
	transport http.RoundTripper
	peers     []string
	mu        sync.RWMutex // guards peers
	hashMap   *consistenthash.Map
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

	c.SetPool(c.peers...)
	return c
}

// SetPool updates the client's peers list. Each peer should
// be a valid base URL, for example "http://example.net:8000".
func (c *Client) SetPool(peers ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.peers = peers
	c.hashMap = consistenthash.New(c.replicas, c.hashFn)
	c.hashMap.Add(c.peers...)
}

// HTTPClient returns an http.Client that uses the Client as its transport.
func (c *Client) HTTPClient() *http.Client {
	cl := new(http.Client)
	*cl = *http.DefaultClient
	cl.Transport = c
	return cl
}

// RoundTrip makes the request go through one of the peer. Since Client
// implements the Roundtripper interface, it can be used as a transport.
func (c *Client) RoundTrip(req *http.Request) (*http.Response, error) {
	peer := c.choosePeer(req.URL.String())
	return c.roundTripTo(peer, req)
}

func (c *Client) choosePeer(url string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.hashMap.Get(url)
}

func (c *Client) roundTripTo(peer string, req *http.Request) (*http.Response, error) {
	query := c.peerHandlerURL(peer, req.URL.String())

	cpy := clone(req) // per RoundTripper contract
	cpy.URL = query
	cpy.Host = query.Host

	return c.transport.RoundTrip(cpy)
}

func (c *Client) peerHandlerURL(peer string, origin string) *url.URL {
	u, _ := url.Parse(peer)

	u.Path = c.path
	u.RawQuery = "q=" + url.QueryEscape(origin)

	return u
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

// WithPool lets you configure the client's list of peers.
// Defaults to nil. See Client.SetPool(...).
func WithPool(peers ...string) func(*Client) {
	return func(c *Client) {
		c.peers = peers
	}
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
