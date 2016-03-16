// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.6

package http2

import (
	"crypto/tls"
	"fmt"
	"net/http"
)

func configureTransport(t1 *http.Transport) error {
	connPool := new(clientConnPool)
	t2 := &Transport{ConnPool: noDialClientConnPool{connPool}}
	if err := registerHTTPSProtocol(t1, noDialH2RoundTripper{t2}); err != nil {
		return err
	}
	if t1.TLSClientConfig == nil {
		t1.TLSClientConfig = new(tls.Config)
	}
	if !strSliceContains(t1.TLSClientConfig.NextProtos, "h2") {
		t1.TLSClientConfig.NextProtos = append([]string{"h2"}, t1.TLSClientConfig.NextProtos...)
	}
	if !strSliceContains(t1.TLSClientConfig.NextProtos, "http/1.1") {
		t1.TLSClientConfig.NextProtos = append(t1.TLSClientConfig.NextProtos, "http/1.1")
	}
	upgradeFn := func(authority string, c *tls.Conn) http.RoundTripper {
		cc, err := t2.NewClientConn(c)
		if err != nil {
			c.Close()
			return erringRoundTripper{err}
		}
		connPool.addConn(authorityAddr(authority), cc)
		return t2
	}
	if m := t1.TLSNextProto; len(m) == 0 {
		t1.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{
			"h2": upgradeFn,
		}
	} else {
		m["h2"] = upgradeFn
	}
	return nil
}

// registerHTTPSProtocol calls Transport.RegisterProtocol but
// convering panics into errors.
func registerHTTPSProtocol(t *http.Transport, rt http.RoundTripper) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	t.RegisterProtocol("https", rt)
	return nil
}

// noDialClientConnPool is an implementation of http2.ClientConnPool
// which never dials.  We let the HTTP/1.1 client dial and use its TLS
// connection instead.
type noDialClientConnPool struct{ *clientConnPool }

func (p noDialClientConnPool) GetClientConn(req *http.Request, addr string) (*ClientConn, error) {
	const doDial = false
	return p.getClientConn(req, addr, doDial)
}

// noDialH2RoundTripper is a RoundTripper which only tries to complete the request
// if there's already has a cached connection to the host.
type noDialH2RoundTripper struct{ t *Transport }

func (rt noDialH2RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := rt.t.RoundTrip(req)
	if err == ErrNoCachedConn {
		return nil, http.ErrSkipAltProtocol
	}
	return res, err
}
