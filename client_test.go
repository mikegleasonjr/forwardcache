package forwardcache

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestClient(t *testing.T) {
	hash := newHashMock().
		with("http://a.com:3000", 0).
		with("http://b.com:3000", 1).
		with("http://c.com:3000", 2).
		with("http://some.url/res-a.js", 0).
		with("http://some.url/res-b.js", 1).
		with("http://some.url/res-c.js", 2)

	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		res := okResponse()
		res.Header.Set("X-Requested-URL", req.URL.String())
		return res, nil
	})

	client := NewClient(
		WithPool("http://a.com:3000", "http://b.com:3000", "http://c.com:3000"),
		WithHashFn(hash.fn),
		WithClientTransport(transport),
		WithPath("/p"),
	).HTTPClient()

	testCases := []struct {
		url  string
		want string
	}{
		{"http://some.url/res-a.js", "http://a.com:3000/p?q=" + url.QueryEscape("http://some.url/res-a.js")},
		{"http://some.url/res-b.js", "http://b.com:3000/p?q=" + url.QueryEscape("http://some.url/res-b.js")},
		{"http://some.url/res-c.js", "http://c.com:3000/p?q=" + url.QueryEscape("http://some.url/res-c.js")},
	}
	for _, tC := range testCases {
		t.Run(tC.url, func(t *testing.T) {
			res, err := client.Get(tC.url)
			io.Copy(ioutil.Discard, res.Body)
			res.Body.Close()

			if err != nil {
				t.Fatalf("unexpected error: got %q, want <nil>", err)
			}

			if got := res.Header.Get("X-Requested-URL"); got != tC.want {
				t.Fatalf("malformed request to peer: got %q, want %q", got, tC.want)
			}
		})
	}
}

func ExampleNewClient() {
	client := NewClient(WithPool("http://10.0.1.1:3000", "http://10.0.1.2:3000"))

	// -then-

	http.DefaultTransport = client
	http.Get("https://...js/1.5.7/angular.min.js")

	// -or-

	http.DefaultClient = client.HTTPClient()
	http.Get("https://...js/1.5.7/angular.min.js")

	// -or-

	c := client.HTTPClient()
	c.Get("https://...js/1.5.7/angular.min.js")
}

func BenchmarkClient(b *testing.B) {
	body := strings.NewReader("OK")
	res := okResponse()
	peer := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		body.Seek(0, io.SeekStart)
		res.Body = ioutil.NopCloser(body)
		res.Request = req
		return res, nil
	})

	client := NewClient(
		WithClientTransport(peer),
		WithPool("http://localhost"),
	).HTTPClient()

	req, _ := http.NewRequest("GET", "http://cdn.com/jquery.js", nil)
	buff := make([]byte, 32*1024)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, _ := client.Do(req)
		io.CopyBuffer(ioutil.Discard, res.Body, buff)
		res.Body.Close()
	}
}
