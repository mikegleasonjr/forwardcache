## Change Log

v2.0.0 - 20/10/2016

* Cleaner API
* Performance improvements (`http.BufferPool`)
* Faster tests (using mocked transports instead of http servers)

v1.0.1 - 20/10/2016

* Can now specify custom `http.Transport`s
* We are now using functional options
  * see [http://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis][fo]
* API breaking change: `client.Client` is now `client.HTTPClient` (sorry I know I bumped to 1.0.0 yesterday)

v1.0.0 - 19/10/2016

* Can now use a nonparticipating client (a client that is not a peer in the pool)



[fo]: http://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis  "gregjones/httpcache"
