package exporter

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"

	"github.com/mholt/caddy/caddyhttp/fastcgi"
)

// FastCGITransport is an implementation of RoundTripper that supports FastCGI.
type FastCGITransport struct {
	Network string
	Address string
}

func (*FastCGITransport) timeout(req *http.Request) time.Duration {
	t, ok := req.Context().Deadline()
	if !ok {
		return 0 // no timeout
	}
	return t.Sub(time.Now())
}

// RoundTrip implements the RoundTripper interface.
func (t *FastCGITransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c, err := fastcgi.DialContext(req.Context(), t.Network, t.Address)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	c.SetSendTimeout(t.timeout(req))
	c.SetReadTimeout(t.timeout(req))

	params := map[string]string{
		"SCRIPT_FILENAME": req.URL.Path,
		"SCRIPT_NAME":     req.URL.Path,
	}
	resp, err := c.Get(params, req.Body, req.ContentLength)
	if err != nil {
		return nil, errors.Wrap(err, "fastcgi get failed")
	}
	body := resp.Body
	defer body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(body); err != nil {
		return nil, err
	}
	resp.Body = ioutil.NopCloser(&buf)
	return resp, nil
}
