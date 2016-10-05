# forwardcache

[![Build Status](https://travis-ci.org/mikegleasonjr/forwardcache.svg?branch=master)](https://travis-ci.org/mikegleasonjr/forwardcache) [![GoDoc](http://godoc.org/github.com/mikegleasonjr/forwardcache?status.svg)](http://godoc.org/github.com/mikegleasonjr/forwardcache)

A distributed forward caching proxy for Go's http.Client using [httpcache][httpcache] and heavily inspired by [groupcache][groupcache]. Backed by a lot of existing cache [backends][backends] thanks to httpcache. A per host LRU algorithm is provided to optionally front any existing cache. Like groupcache, forwardcache "is a client library as well as a server. It connects to its own peers."

Docs on [godoc.org][godoc]

<pre>
+---------------------------------+
|                                 |
|         origin servers          |
|                                 |
+--+-------+-------+-----------+--+
   ^       ^       ^           ^
   |       |       |           |
   |       |       |           |
+--+--+ +--+--+ +--+--+     +--+--+
|     | |     | |     |     |     |
|  1  +-+  2  +-+  3  +--|--+  N  |
|     | |     | |     |     |     |
+-----+ +-----+ +-----+     +-----+
|cache| |cache| |cache|     |cache|
+-----+ +-----+ +-----+     +-----+
</pre>

## Requirements

* Go 1.7 (using request's context)

## Motivation

* Needed requests to be cached
* Needed bandwidth to be spread among N peers

## Process

When making a request through the pool...

1. the requested url is hashed to determine which peer is responisble to proxy the request to the origin
2. the peer is called, if the request is cached and valid, it is returned without contacting the origin
3. if the request is not cached, the request is fetched from the origin and cached if cacheable before being returned back to the client

## Example

```go
cache := httpcache.Cache(httpcache.NewMemoryCache())
cache = lru.New(cache, 32<<20)

pool := forwardcache.NewPool("http://10.0.1.1", cache)
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

http.ListenAndServe(":3000", pool.LocalProxy())
```

## Todo

* Use [x/sync/singleflight][singleflight] to avoid simultaneous queries to origins
* Use a configurable user agent on proxies when requesting origins

## Licence

Apache 2.0 (see LICENSE and NOTICE)






[httpcache]: https://github.com/gregjones/httpcache  "gregjones/httpcache"
[backends]: https://github.com/gregjones/httpcache#cache-backends  "cache backends"
[groupcache]: https://github.com/gregjones/httpcache#cache-backends  "golang/groupcache"
[singleflight]: https://godoc.org/golang.org/x/sync/singleflight "x/sync/singleflight"
[godoc]: http://godoc.org/github.com/mikegleasonjr/forwardcache "mikegleasonjr/forwardcache"
