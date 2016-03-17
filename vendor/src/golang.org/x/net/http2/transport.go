// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Transport code.

package http2

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/http2/hpack"
)

const (
	// transportDefaultConnFlow is how many connection-level flow control
	// tokens we give the server at start-up, past the default 64k.
	transportDefaultConnFlow = 1 << 30

	// transportDefaultStreamFlow is how many stream-level flow
	// control tokens we announce to the peer, and how many bytes
	// we buffer per stream.
	transportDefaultStreamFlow = 4 << 20

	// transportDefaultStreamMinRefresh is the minimum number of bytes we'll send
	// a stream-level WINDOW_UPDATE for at a time.
	transportDefaultStreamMinRefresh = 4 << 10
)

// Transport is an HTTP/2 Transport.
//
// A Transport internally caches connections to servers. It is safe
// for concurrent use by multiple goroutines.
type Transport struct {
	// DialTLS specifies an optional dial function for creating
	// TLS connections for requests.
	//
	// If DialTLS is nil, tls.Dial is used.
	//
	// If the returned net.Conn has a ConnectionState method like tls.Conn,
	// it will be used to set http.Response.TLS.
	DialTLS func(network, addr string, cfg *tls.Config) (net.Conn, error)

	// TLSClientConfig specifies the TLS configuration to use with
	// tls.Client. If nil, the default configuration is used.
	TLSClientConfig *tls.Config

	// ConnPool optionally specifies an alternate connection pool to use.
	// If nil, the default is used.
	ConnPool ClientConnPool

	// DisableCompression, if true, prevents the Transport from
	// requesting compression with an "Accept-Encoding: gzip"
	// request header when the Request contains no existing
	// Accept-Encoding value. If the Transport requests gzip on
	// its own and gets a gzipped response, it's transparently
	// decoded in the Response.Body. However, if the user
	// explicitly requested gzip it is not automatically
	// uncompressed.
	DisableCompression bool

	connPoolOnce  sync.Once
	connPoolOrDef ClientConnPool // non-nil version of ConnPool
}

func (t *Transport) disableCompression() bool {
	if t.DisableCompression {
		return true
	}
	// TODO: also disable if this transport is somehow linked to an http1 Transport
	// and it's configured there?
	return false
}

var errTransportVersion = errors.New("http2: ConfigureTransport is only supported starting at Go 1.6")

// ConfigureTransport configures a net/http HTTP/1 Transport to use HTTP/2.
// It requires Go 1.6 or later and returns an error if the net/http package is too old
// or if t1 has already been HTTP/2-enabled.
func ConfigureTransport(t1 *http.Transport) error {
	return configureTransport(t1) // in configure_transport.go (go1.6) or go15.go
}

func (t *Transport) connPool() ClientConnPool {
	t.connPoolOnce.Do(t.initConnPool)
	return t.connPoolOrDef
}

func (t *Transport) initConnPool() {
	if t.ConnPool != nil {
		t.connPoolOrDef = t.ConnPool
	} else {
		t.connPoolOrDef = &clientConnPool{t: t}
	}
}

// ClientConn is the state of a single HTTP/2 client connection to an
// HTTP/2 server.
type ClientConn struct {
	t        *Transport
	tconn    net.Conn             // usually *tls.Conn, except specialized impls
	tlsState *tls.ConnectionState // nil only for specialized impls

	// readLoop goroutine fields:
	readerDone chan struct{} // closed on error
	readerErr  error         // set before readerDone is closed

	mu           sync.Mutex // guards following
	cond         *sync.Cond // hold mu; broadcast on flow/closed changes
	flow         flow       // our conn-level flow control quota (cs.flow is per stream)
	inflow       flow       // peer's conn-level flow control
	closed       bool
	goAway       *GoAwayFrame             // if non-nil, the GoAwayFrame we received
	streams      map[uint32]*clientStream // client-initiated
	nextStreamID uint32
	bw           *bufio.Writer
	br           *bufio.Reader
	fr           *Framer
	// Settings from peer:
	maxFrameSize         uint32
	maxConcurrentStreams uint32
	initialWindowSize    uint32
	hbuf                 bytes.Buffer // HPACK encoder writes into this
	henc                 *hpack.Encoder
	freeBuf              [][]byte

	wmu  sync.Mutex // held while writing; acquire AFTER wmu if holding both
	werr error      // first write error that has occurred
}

// clientStream is the state for a single HTTP/2 stream. One of these
// is created for each Transport.RoundTrip call.
type clientStream struct {
	cc            *ClientConn
	req           *http.Request
	ID            uint32
	resc          chan resAndError
	bufPipe       pipe // buffered pipe with the flow-controlled response payload
	requestedGzip bool

	flow        flow  // guarded by cc.mu
	inflow      flow  // guarded by cc.mu
	bytesRemain int64 // -1 means unknown; owned by transportResponseBody.Read
	readErr     error // sticky read error; owned by transportResponseBody.Read
	stopReqBody bool  // stop writing req body; guarded by cc.mu

	peerReset chan struct{} // closed on peer reset
	resetErr  error         // populated before peerReset is closed

	// owned by clientConnReadLoop:
	headersDone  bool // got HEADERS w/ END_HEADERS
	trailersDone bool // got second HEADERS frame w/ END_HEADERS

	trailer    http.Header // accumulated trailers
	resTrailer http.Header // client's Response.Trailer
}

// awaitRequestCancel runs in its own goroutine and waits for the user's
func (cs *clientStream) awaitRequestCancel(cancel <-chan struct{}) {
	if cancel == nil {
		return
	}
	select {
	case <-cancel:
		cs.bufPipe.CloseWithError(errRequestCanceled)
	case <-cs.bufPipe.Done():
	}
}

// checkReset reports any error sent in a RST_STREAM frame by the
// server.
func (cs *clientStream) checkReset() error {
	select {
	case <-cs.peerReset:
		return cs.resetErr
	default:
		return nil
	}
}

func (cs *clientStream) abortRequestBodyWrite() {
	cc := cs.cc
	cc.mu.Lock()
	cs.stopReqBody = true
	cc.cond.Broadcast()
	cc.mu.Unlock()
}

type stickyErrWriter struct {
	w   io.Writer
	err *error
}

func (sew stickyErrWriter) Write(p []byte) (n int, err error) {
	if *sew.err != nil {
		return 0, *sew.err
	}
	n, err = sew.w.Write(p)
	*sew.err = err
	return
}

var ErrNoCachedConn = errors.New("http2: no cached connection was available")

// RoundTripOpt are options for the Transport.RoundTripOpt method.
type RoundTripOpt struct {
	// OnlyCachedConn controls whether RoundTripOpt may
	// create a new TCP connection. If set true and
	// no cached connection is available, RoundTripOpt
	// will return ErrNoCachedConn.
	OnlyCachedConn bool
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.RoundTripOpt(req, RoundTripOpt{})
}

// authorityAddr returns a given authority (a host/IP, or host:port / ip:port)
// and returns a host:port. The port 443 is added if needed.
func authorityAddr(authority string) (addr string) {
	if _, _, err := net.SplitHostPort(authority); err == nil {
		return authority
	}
	return net.JoinHostPort(authority, "443")
}

// RoundTripOpt is like RoundTrip, but takes options.
func (t *Transport) RoundTripOpt(req *http.Request, opt RoundTripOpt) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return nil, errors.New("http2: unsupported scheme")
	}

	addr := authorityAddr(req.URL.Host)
	for {
		cc, err := t.connPool().GetClientConn(req, addr)
		if err != nil {
			t.vlogf("failed to get client conn: %v", err)
			return nil, err
		}
		res, err := cc.RoundTrip(req)
		if shouldRetryRequest(req, err) {
			continue
		}
		if err != nil {
			t.vlogf("RoundTrip failure: %v", err)
			return nil, err
		}
		return res, nil
	}
}

// CloseIdleConnections closes any connections which were previously
// connected from previous requests but are now sitting idle.
// It does not interrupt any connections currently in use.
func (t *Transport) CloseIdleConnections() {
	if cp, ok := t.connPool().(*clientConnPool); ok {
		cp.closeIdleConnections()
	}
}

var (
	errClientConnClosed   = errors.New("http2: client conn is closed")
	errClientConnUnusable = errors.New("http2: client conn not usable")
)

func shouldRetryRequest(req *http.Request, err error) bool {
	// TODO: retry GET requests (no bodies) more aggressively, if shutdown
	// before response.
	return err == errClientConnUnusable
}

func (t *Transport) dialClientConn(addr string) (*ClientConn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	tconn, err := t.dialTLS()("tcp", addr, t.newTLSConfig(host))
	if err != nil {
		return nil, err
	}
	return t.NewClientConn(tconn)
}

func (t *Transport) newTLSConfig(host string) *tls.Config {
	cfg := new(tls.Config)
	if t.TLSClientConfig != nil {
		*cfg = *t.TLSClientConfig
	}
	cfg.NextProtos = []string{NextProtoTLS} // TODO: don't override if already in list
	cfg.ServerName = host
	return cfg
}

func (t *Transport) dialTLS() func(string, string, *tls.Config) (net.Conn, error) {
	if t.DialTLS != nil {
		return t.DialTLS
	}
	return t.dialTLSDefault
}

func (t *Transport) dialTLSDefault(network, addr string, cfg *tls.Config) (net.Conn, error) {
	cn, err := tls.Dial(network, addr, cfg)
	if err != nil {
		return nil, err
	}
	if err := cn.Handshake(); err != nil {
		return nil, err
	}
	if !cfg.InsecureSkipVerify {
		if err := cn.VerifyHostname(cfg.ServerName); err != nil {
			return nil, err
		}
	}
	state := cn.ConnectionState()
	if p := state.NegotiatedProtocol; p != NextProtoTLS {
		return nil, fmt.Errorf("http2: unexpected ALPN protocol %q; want %q", p, NextProtoTLS)
	}
	if !state.NegotiatedProtocolIsMutual {
		return nil, errors.New("http2: could not negotiate protocol mutually")
	}
	return cn, nil
}

func (t *Transport) NewClientConn(c net.Conn) (*ClientConn, error) {
	if VerboseLogs {
		t.vlogf("creating client conn to %v", c.RemoteAddr())
	}
	if _, err := c.Write(clientPreface); err != nil {
		t.vlogf("client preface write error: %v", err)
		return nil, err
	}

	cc := &ClientConn{
		t:                    t,
		tconn:                c,
		readerDone:           make(chan struct{}),
		nextStreamID:         1,
		maxFrameSize:         16 << 10, // spec default
		initialWindowSize:    65535,    // spec default
		maxConcurrentStreams: 1000,     // "infinite", per spec. 1000 seems good enough.
		streams:              make(map[uint32]*clientStream),
	}
	cc.cond = sync.NewCond(&cc.mu)
	cc.flow.add(int32(initialWindowSize))

	// TODO: adjust this writer size to account for frame size +
	// MTU + crypto/tls record padding.
	cc.bw = bufio.NewWriter(stickyErrWriter{c, &cc.werr})
	cc.br = bufio.NewReader(c)
	cc.fr = NewFramer(cc.bw, cc.br)
	cc.henc = hpack.NewEncoder(&cc.hbuf)

	type connectionStater interface {
		ConnectionState() tls.ConnectionState
	}
	if cs, ok := c.(connectionStater); ok {
		state := cs.ConnectionState()
		cc.tlsState = &state
	}

	cc.fr.WriteSettings(
		Setting{ID: SettingEnablePush, Val: 0},
		Setting{ID: SettingInitialWindowSize, Val: transportDefaultStreamFlow},
	)
	cc.fr.WriteWindowUpdate(0, transportDefaultConnFlow)
	cc.inflow.add(transportDefaultConnFlow + initialWindowSize)
	cc.bw.Flush()
	if cc.werr != nil {
		return nil, cc.werr
	}

	// Read the obligatory SETTINGS frame
	f, err := cc.fr.ReadFrame()
	if err != nil {
		return nil, err
	}
	sf, ok := f.(*SettingsFrame)
	if !ok {
		return nil, fmt.Errorf("expected settings frame, got: %T", f)
	}
	cc.fr.WriteSettingsAck()
	cc.bw.Flush()

	sf.ForeachSetting(func(s Setting) error {
		switch s.ID {
		case SettingMaxFrameSize:
			cc.maxFrameSize = s.Val
		case SettingMaxConcurrentStreams:
			cc.maxConcurrentStreams = s.Val
		case SettingInitialWindowSize:
			cc.initialWindowSize = s.Val
		default:
			// TODO(bradfitz): handle more
			t.vlogf("Unhandled Setting: %v", s)
		}
		return nil
	})

	go cc.readLoop()
	return cc, nil
}

func (cc *ClientConn) setGoAway(f *GoAwayFrame) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.goAway = f
}

func (cc *ClientConn) CanTakeNewRequest() bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.canTakeNewRequestLocked()
}

func (cc *ClientConn) canTakeNewRequestLocked() bool {
	return cc.goAway == nil &&
		int64(len(cc.streams)+1) < int64(cc.maxConcurrentStreams) &&
		cc.nextStreamID < 2147483647
}

func (cc *ClientConn) closeIfIdle() {
	cc.mu.Lock()
	if len(cc.streams) > 0 {
		cc.mu.Unlock()
		return
	}
	cc.closed = true
	// TODO: do clients send GOAWAY too? maybe? Just Close:
	cc.mu.Unlock()

	cc.tconn.Close()
}

const maxAllocFrameSize = 512 << 10

// frameBuffer returns a scratch buffer suitable for writing DATA frames.
// They're capped at the min of the peer's max frame size or 512KB
// (kinda arbitrarily), but definitely capped so we don't allocate 4GB
// bufers.
func (cc *ClientConn) frameScratchBuffer() []byte {
	cc.mu.Lock()
	size := cc.maxFrameSize
	if size > maxAllocFrameSize {
		size = maxAllocFrameSize
	}
	for i, buf := range cc.freeBuf {
		if len(buf) >= int(size) {
			cc.freeBuf[i] = nil
			cc.mu.Unlock()
			return buf[:size]
		}
	}
	cc.mu.Unlock()
	return make([]byte, size)
}

func (cc *ClientConn) putFrameScratchBuffer(buf []byte) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	const maxBufs = 4 // arbitrary; 4 concurrent requests per conn? investigate.
	if len(cc.freeBuf) < maxBufs {
		cc.freeBuf = append(cc.freeBuf, buf)
		return
	}
	for i, old := range cc.freeBuf {
		if old == nil {
			cc.freeBuf[i] = buf
			return
		}
	}
	// forget about it.
}

// errRequestCanceled is a copy of net/http's errRequestCanceled because it's not
// exported. At least they'll be DeepEqual for h1-vs-h2 comparisons tests.
var errRequestCanceled = errors.New("net/http: request canceled")

func commaSeparatedTrailers(req *http.Request) (string, error) {
	keys := make([]string, 0, len(req.Trailer))
	for k := range req.Trailer {
		k = http.CanonicalHeaderKey(k)
		switch k {
		case "Transfer-Encoding", "Trailer", "Content-Length":
			return "", &badStringError{"invalid Trailer key", k}
		}
		keys = append(keys, k)
	}
	if len(keys) > 0 {
		sort.Strings(keys)
		// TODO: could do better allocation-wise here, but trailers are rare,
		// so being lazy for now.
		return strings.Join(keys, ","), nil
	}
	return "", nil
}

func (cc *ClientConn) RoundTrip(req *http.Request) (*http.Response, error) {
	trailers, err := commaSeparatedTrailers(req)
	if err != nil {
		return nil, err
	}
	hasTrailers := trailers != ""

	cc.mu.Lock()
	if cc.closed || !cc.canTakeNewRequestLocked() {
		cc.mu.Unlock()
		return nil, errClientConnUnusable
	}

	cs := cc.newStream()
	cs.req = req
	hasBody := req.Body != nil

	// TODO(bradfitz): this is a copy of the logic in net/http. Unify somewhere?
	if !cc.t.disableCompression() &&
		req.Header.Get("Accept-Encoding") == "" &&
		req.Header.Get("Range") == "" &&
		req.Method != "HEAD" {
		// Request gzip only, not deflate. Deflate is ambiguous and
		// not as universally supported anyway.
		// See: http://www.gzip.org/zlib/zlib_faq.html#faq38
		//
		// Note that we don't request this for HEAD requests,
		// due to a bug in nginx:
		//   http://trac.nginx.org/nginx/ticket/358
		//   https://golang.org/issue/5522
		//
		// We don't request gzip if the request is for a range, since
		// auto-decoding a portion of a gzipped document will just fail
		// anyway. See https://golang.org/issue/8923
		cs.requestedGzip = true
	}

	// we send: HEADERS{1}, CONTINUATION{0,} + DATA{0,}
	hdrs := cc.encodeHeaders(req, cs.requestedGzip, trailers)
	cc.wmu.Lock()
	endStream := !hasBody && !hasTrailers
	werr := cc.writeHeaders(cs.ID, endStream, hdrs)
	cc.wmu.Unlock()
	cc.mu.Unlock()

	if werr != nil {
		return nil, werr
	}

	var bodyCopyErrc chan error // result of body copy
	if hasBody {
		bodyCopyErrc = make(chan error, 1)
		go func() {
			bodyCopyErrc <- cs.writeRequestBody(req.Body)
		}()
	}

	for {
		select {
		case re := <-cs.resc:
			res := re.res
			if re.err != nil || res.StatusCode > 299 {
				// On error or status code 3xx, 4xx, 5xx, etc abort any
				// ongoing write, assuming that the server doesn't care
				// about our request body. If the server replied with 1xx or
				// 2xx, however, then assume the server DOES potentially
				// want our body (e.g. full-duplex streaming:
				// golang.org/issue/13444). If it turns out the server
				// doesn't, they'll RST_STREAM us soon enough.  This is a
				// heuristic to avoid adding knobs to Transport.  Hopefully
				// we can keep it.
				cs.abortRequestBodyWrite()
			}
			if re.err != nil {
				return nil, re.err
			}
			res.Request = req
			res.TLS = cc.tlsState
			return res, nil
		case <-requestCancel(req):
			cs.abortRequestBodyWrite()
			return nil, errRequestCanceled
		case <-cs.peerReset:
			return nil, cs.resetErr
		case err := <-bodyCopyErrc:
			if err != nil {
				return nil, err
			}
		}
	}
}

// requires cc.wmu be held
func (cc *ClientConn) writeHeaders(streamID uint32, endStream bool, hdrs []byte) error {
	first := true // first frame written (HEADERS is first, then CONTINUATION)
	frameSize := int(cc.maxFrameSize)
	for len(hdrs) > 0 && cc.werr == nil {
		chunk := hdrs
		if len(chunk) > frameSize {
			chunk = chunk[:frameSize]
		}
		hdrs = hdrs[len(chunk):]
		endHeaders := len(hdrs) == 0
		if first {
			cc.fr.WriteHeaders(HeadersFrameParam{
				StreamID:      streamID,
				BlockFragment: chunk,
				EndStream:     endStream,
				EndHeaders:    endHeaders,
			})
			first = false
		} else {
			cc.fr.WriteContinuation(streamID, endHeaders, chunk)
		}
	}
	// TODO(bradfitz): this Flush could potentially block (as
	// could the WriteHeaders call(s) above), which means they
	// wouldn't respond to Request.Cancel being readable. That's
	// rare, but this should probably be in a goroutine.
	cc.bw.Flush()
	return cc.werr
}

// errAbortReqBodyWrite is an internal error value.
// It doesn't escape to callers.
var errAbortReqBodyWrite = errors.New("http2: aborting request body write")

func (cs *clientStream) writeRequestBody(body io.ReadCloser) (err error) {
	cc := cs.cc
	sentEnd := false // whether we sent the final DATA frame w/ END_STREAM
	buf := cc.frameScratchBuffer()
	defer cc.putFrameScratchBuffer(buf)

	defer func() {
		// TODO: write h12Compare test showing whether
		// Request.Body is closed by the Transport,
		// and in multiple cases: server replies <=299 and >299
		// while still writing request body
		cerr := body.Close()
		if err == nil {
			err = cerr
		}
	}()

	req := cs.req
	hasTrailers := req.Trailer != nil

	var sawEOF bool
	for !sawEOF {
		n, err := body.Read(buf)
		if err == io.EOF {
			sawEOF = true
			err = nil
		} else if err != nil {
			return err
		}

		remain := buf[:n]
		for len(remain) > 0 && err == nil {
			var allowed int32
			allowed, err = cs.awaitFlowControl(len(remain))
			if err != nil {
				return err
			}
			cc.wmu.Lock()
			data := remain[:allowed]
			remain = remain[allowed:]
			sentEnd = sawEOF && len(remain) == 0 && !hasTrailers
			err = cc.fr.WriteData(cs.ID, sentEnd, data)
			if err == nil {
				// TODO(bradfitz): this flush is for latency, not bandwidth.
				// Most requests won't need this. Make this opt-in or opt-out?
				// Use some heuristic on the body type? Nagel-like timers?
				// Based on 'n'? Only last chunk of this for loop, unless flow control
				// tokens are low? For now, always:
				err = cc.bw.Flush()
			}
			cc.wmu.Unlock()
		}
		if err != nil {
			return err
		}
	}

	cc.wmu.Lock()
	if !sentEnd {
		var trls []byte
		if hasTrailers {
			cc.mu.Lock()
			trls = cc.encodeTrailers(req)
			cc.mu.Unlock()
		}

		// Avoid forgetting to send an END_STREAM if the encoded
		// trailers are 0 bytes. Both results produce and END_STREAM.
		if len(trls) > 0 {
			err = cc.writeHeaders(cs.ID, true, trls)
		} else {
			err = cc.fr.WriteData(cs.ID, true, nil)
		}
	}
	if ferr := cc.bw.Flush(); ferr != nil && err == nil {
		err = ferr
	}
	cc.wmu.Unlock()

	return err
}

// awaitFlowControl waits for [1, min(maxBytes, cc.cs.maxFrameSize)] flow
// control tokens from the server.
// It returns either the non-zero number of tokens taken or an error
// if the stream is dead.
func (cs *clientStream) awaitFlowControl(maxBytes int) (taken int32, err error) {
	cc := cs.cc
	cc.mu.Lock()
	defer cc.mu.Unlock()
	for {
		if cc.closed {
			return 0, errClientConnClosed
		}
		if cs.stopReqBody {
			return 0, errAbortReqBodyWrite
		}
		if err := cs.checkReset(); err != nil {
			return 0, err
		}
		if a := cs.flow.available(); a > 0 {
			take := a
			if int(take) > maxBytes {

				take = int32(maxBytes) // can't truncate int; take is int32
			}
			if take > int32(cc.maxFrameSize) {
				take = int32(cc.maxFrameSize)
			}
			cs.flow.take(take)
			return take, nil
		}
		cc.cond.Wait()
	}
}

type badStringError struct {
	what string
	str  string
}

func (e *badStringError) Error() string { return fmt.Sprintf("%s %q", e.what, e.str) }

// requires cc.mu be held.
func (cc *ClientConn) encodeHeaders(req *http.Request, addGzipHeader bool, trailers string) []byte {
	cc.hbuf.Reset()

	// TODO(bradfitz): figure out :authority-vs-Host stuff between http2 and Go
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	// 8.1.2.3 Request Pseudo-Header Fields
	// The :path pseudo-header field includes the path and query parts of the
	// target URI (the path-absolute production and optionally a '?' character
	// followed by the query production (see Sections 3.3 and 3.4 of
	// [RFC3986]).
	cc.writeHeader(":authority", host) // probably not right for all sites
	cc.writeHeader(":method", req.Method)
	cc.writeHeader(":path", req.URL.RequestURI())
	cc.writeHeader(":scheme", "https")
	if trailers != "" {
		cc.writeHeader("trailer", trailers)
	}

	for k, vv := range req.Header {
		lowKey := strings.ToLower(k)
		if lowKey == "host" {
			continue
		}
		for _, v := range vv {
			cc.writeHeader(lowKey, v)
		}
	}
	if addGzipHeader {
		cc.writeHeader("accept-encoding", "gzip")
	}
	return cc.hbuf.Bytes()
}

// requires cc.mu be held.
func (cc *ClientConn) encodeTrailers(req *http.Request) []byte {
	cc.hbuf.Reset()
	for k, vv := range req.Trailer {
		// Transfer-Encoding, etc.. have already been filter at the
		// start of RoundTrip
		lowKey := strings.ToLower(k)
		for _, v := range vv {
			cc.writeHeader(lowKey, v)
		}
	}
	return cc.hbuf.Bytes()
}

func (cc *ClientConn) writeHeader(name, value string) {
	cc.henc.WriteField(hpack.HeaderField{Name: name, Value: value})
}

type resAndError struct {
	res *http.Response
	err error
}

// requires cc.mu be held.
func (cc *ClientConn) newStream() *clientStream {
	cs := &clientStream{
		cc:        cc,
		ID:        cc.nextStreamID,
		resc:      make(chan resAndError, 1),
		peerReset: make(chan struct{}),
	}
	cs.flow.add(int32(cc.initialWindowSize))
	cs.flow.setConnFlow(&cc.flow)
	cs.inflow.add(transportDefaultStreamFlow)
	cs.inflow.setConnFlow(&cc.inflow)
	cc.nextStreamID += 2
	cc.streams[cs.ID] = cs
	return cs
}

func (cc *ClientConn) streamByID(id uint32, andRemove bool) *clientStream {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cs := cc.streams[id]
	if andRemove {
		delete(cc.streams, id)
	}
	return cs
}

// clientConnReadLoop is the state owned by the clientConn's frame-reading readLoop.
type clientConnReadLoop struct {
	cc        *ClientConn
	activeRes map[uint32]*clientStream // keyed by streamID

	hdec *hpack.Decoder

	// Fields reset on each HEADERS:
	nextRes      *http.Response
	sawRegHeader bool  // saw non-pseudo header
	reqMalformed error // non-nil once known to be malformed
}

// readLoop runs in its own goroutine and reads and dispatches frames.
func (cc *ClientConn) readLoop() {
	rl := &clientConnReadLoop{
		cc:        cc,
		activeRes: make(map[uint32]*clientStream),
	}
	// TODO: figure out henc size
	rl.hdec = hpack.NewDecoder(initialHeaderTableSize, rl.onNewHeaderField)

	defer rl.cleanup()
	cc.readerErr = rl.run()
	if ce, ok := cc.readerErr.(ConnectionError); ok {
		cc.wmu.Lock()
		cc.fr.WriteGoAway(0, ErrCode(ce), nil)
		cc.wmu.Unlock()
	}
}

func (rl *clientConnReadLoop) cleanup() {
	cc := rl.cc
	defer cc.tconn.Close()
	defer cc.t.connPool().MarkDead(cc)
	defer close(cc.readerDone)

	// Close any response bodies if the server closes prematurely.
	// TODO: also do this if we've written the headers but not
	// gotten a response yet.
	err := cc.readerErr
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	cc.mu.Lock()
	for _, cs := range rl.activeRes {
		cs.bufPipe.CloseWithError(err)
	}
	for _, cs := range cc.streams {
		select {
		case cs.resc <- resAndError{err: err}:
		default:
		}
	}
	cc.closed = true
	cc.cond.Broadcast()
	cc.mu.Unlock()
}

func (rl *clientConnReadLoop) run() error {
	cc := rl.cc
	for {
		f, err := cc.fr.ReadFrame()
		if err != nil {
			cc.vlogf("Transport readFrame error: (%T) %v", err, err)
		}
		if se, ok := err.(StreamError); ok {
			// TODO: deal with stream errors from the framer.
			return se
		} else if err != nil {
			return err
		}
		cc.vlogf("Transport received %v: %#v", f.Header(), f)

		switch f := f.(type) {
		case *HeadersFrame:
			err = rl.processHeaders(f)
		case *ContinuationFrame:
			err = rl.processContinuation(f)
		case *DataFrame:
			err = rl.processData(f)
		case *GoAwayFrame:
			err = rl.processGoAway(f)
		case *RSTStreamFrame:
			err = rl.processResetStream(f)
		case *SettingsFrame:
			err = rl.processSettings(f)
		case *PushPromiseFrame:
			err = rl.processPushPromise(f)
		case *WindowUpdateFrame:
			err = rl.processWindowUpdate(f)
		case *PingFrame:
			err = rl.processPing(f)
		default:
			cc.logf("Transport: unhandled response frame type %T", f)
		}
		if err != nil {
			return err
		}
	}
}

func (rl *clientConnReadLoop) processHeaders(f *HeadersFrame) error {
	rl.sawRegHeader = false
	rl.reqMalformed = nil
	rl.nextRes = &http.Response{
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		Header:     make(http.Header),
	}
	return rl.processHeaderBlockFragment(f.HeaderBlockFragment(), f.StreamID, f.HeadersEnded(), f.StreamEnded())
}

func (rl *clientConnReadLoop) processContinuation(f *ContinuationFrame) error {
	return rl.processHeaderBlockFragment(f.HeaderBlockFragment(), f.StreamID, f.HeadersEnded(), f.StreamEnded())
}

func (rl *clientConnReadLoop) processHeaderBlockFragment(frag []byte, streamID uint32, headersEnded, streamEnded bool) error {
	cc := rl.cc
	cs := cc.streamByID(streamID, streamEnded)
	if cs == nil {
		// We'd get here if we canceled a request while the
		// server was mid-way through replying with its
		// headers. (The case of a CONTINUATION arriving
		// without HEADERS would be rejected earlier by the
		// Framer). So if this was just something we canceled,
		// ignore it.
		return nil
	}
	if cs.headersDone {
		rl.hdec.SetEmitFunc(cs.onNewTrailerField)
	} else {
		rl.hdec.SetEmitFunc(rl.onNewHeaderField)
	}
	_, err := rl.hdec.Write(frag)
	if err != nil {
		return ConnectionError(ErrCodeCompression)
	}
	if err := rl.hdec.Close(); err != nil {
		return ConnectionError(ErrCodeCompression)
	}
	if !headersEnded {
		return nil
	}

	if !cs.headersDone {
		cs.headersDone = true
	} else {
		// We're dealing with trailers. (and specifically the
		// final frame of headers)
		if cs.trailersDone {
			// Too many HEADERS frames for this stream.
			return ConnectionError(ErrCodeProtocol)
		}
		cs.trailersDone = true
		if !streamEnded {
			// We expect that any header block fragment
			// frame for trailers with END_HEADERS also
			// has END_STREAM.
			return ConnectionError(ErrCodeProtocol)
		}
		rl.endStream(cs)
		return nil
	}

	if rl.reqMalformed != nil {
		cs.resc <- resAndError{err: rl.reqMalformed}
		rl.cc.writeStreamReset(cs.ID, ErrCodeProtocol, rl.reqMalformed)
		return nil
	}

	res := rl.nextRes

	if !streamEnded || cs.req.Method == "HEAD" {
		res.ContentLength = -1
		if clens := res.Header["Content-Length"]; len(clens) == 1 {
			if clen64, err := strconv.ParseInt(clens[0], 10, 64); err == nil {
				res.ContentLength = clen64
			} else {
				// TODO: care? unlike http/1, it won't mess up our framing, so it's
				// more safe smuggling-wise to ignore.
			}
		} else if len(clens) > 1 {
			// TODO: care? unlike http/1, it won't mess up our framing, so it's
			// more safe smuggling-wise to ignore.
		}
	}

	if streamEnded {
		res.Body = noBody
	} else {
		buf := new(bytes.Buffer) // TODO(bradfitz): recycle this garbage
		cs.bufPipe = pipe{b: buf}
		cs.bytesRemain = res.ContentLength
		res.Body = transportResponseBody{cs}
		go cs.awaitRequestCancel(requestCancel(cs.req))

		if cs.requestedGzip && res.Header.Get("Content-Encoding") == "gzip" {
			res.Header.Del("Content-Encoding")
			res.Header.Del("Content-Length")
			res.ContentLength = -1
			res.Body = &gzipReader{body: res.Body}
		}
	}

	cs.resTrailer = res.Trailer
	rl.activeRes[cs.ID] = cs
	cs.resc <- resAndError{res: res}
	rl.nextRes = nil // unused now; will be reset next HEADERS frame
	return nil
}

// transportResponseBody is the concrete type of Transport.RoundTrip's
// Response.Body. It is an io.ReadCloser. On Read, it reads from cs.body.
// On Close it sends RST_STREAM if EOF wasn't already seen.
type transportResponseBody struct {
	cs *clientStream
}

func (b transportResponseBody) Read(p []byte) (n int, err error) {
	cs := b.cs
	cc := cs.cc

	if cs.readErr != nil {
		return 0, cs.readErr
	}
	n, err = b.cs.bufPipe.Read(p)
	if cs.bytesRemain != -1 {
		if int64(n) > cs.bytesRemain {
			n = int(cs.bytesRemain)
			if err == nil {
				err = errors.New("net/http: server replied with more than declared Content-Length; truncated")
				cc.writeStreamReset(cs.ID, ErrCodeProtocol, err)
			}
			cs.readErr = err
			return int(cs.bytesRemain), err
		}
		cs.bytesRemain -= int64(n)
		if err == io.EOF && cs.bytesRemain > 0 {
			err = io.ErrUnexpectedEOF
			cs.readErr = err
			return n, err
		}
	}
	if n == 0 {
		// No flow control tokens to send back.
		return
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	var connAdd, streamAdd int32
	// Check the conn-level first, before the stream-level.
	if v := cc.inflow.available(); v < transportDefaultConnFlow/2 {
		connAdd = transportDefaultConnFlow - v
		cc.inflow.add(connAdd)
	}
	if err == nil { // No need to refresh if the stream is over or failed.
		if v := cs.inflow.available(); v < transportDefaultStreamFlow-transportDefaultStreamMinRefresh {
			streamAdd = transportDefaultStreamFlow - v
			cs.inflow.add(streamAdd)
		}
	}
	if connAdd != 0 || streamAdd != 0 {
		cc.wmu.Lock()
		defer cc.wmu.Unlock()
		if connAdd != 0 {
			cc.fr.WriteWindowUpdate(0, mustUint31(connAdd))
		}
		if streamAdd != 0 {
			cc.fr.WriteWindowUpdate(cs.ID, mustUint31(streamAdd))
		}
		cc.bw.Flush()
	}
	return
}

var errClosedResponseBody = errors.New("http2: response body closed")

func (b transportResponseBody) Close() error {
	cs := b.cs
	if cs.bufPipe.Err() != io.EOF {
		// TODO: write test for this
		cs.cc.writeStreamReset(cs.ID, ErrCodeCancel, nil)
	}
	cs.bufPipe.BreakWithError(errClosedResponseBody)
	return nil
}

func (rl *clientConnReadLoop) processData(f *DataFrame) error {
	cc := rl.cc
	cs := cc.streamByID(f.StreamID, f.StreamEnded())
	if cs == nil {
		return nil
	}
	data := f.Data()
	if VerboseLogs {
		rl.cc.logf("DATA: %q", data)
	}

	// Check connection-level flow control.
	cc.mu.Lock()
	if cs.inflow.available() >= int32(len(data)) {
		cs.inflow.take(int32(len(data)))
	} else {
		cc.mu.Unlock()
		return ConnectionError(ErrCodeFlowControl)
	}
	cc.mu.Unlock()

	if _, err := cs.bufPipe.Write(data); err != nil {
		return err
	}

	if f.StreamEnded() {
		rl.endStream(cs)
	}
	return nil
}

func (rl *clientConnReadLoop) endStream(cs *clientStream) {
	// TODO: check that any declared content-length matches, like
	// server.go's (*stream).endStream method.
	cs.bufPipe.closeWithErrorAndCode(io.EOF, cs.copyTrailers)
	delete(rl.activeRes, cs.ID)
}

func (cs *clientStream) copyTrailers() {
	for k, vv := range cs.trailer {
		cs.resTrailer[k] = vv
	}
}

func (rl *clientConnReadLoop) processGoAway(f *GoAwayFrame) error {
	cc := rl.cc
	cc.t.connPool().MarkDead(cc)
	if f.ErrCode != 0 {
		// TODO: deal with GOAWAY more. particularly the error code
		cc.vlogf("transport got GOAWAY with error code = %v", f.ErrCode)
	}
	cc.setGoAway(f)
	return nil
}

func (rl *clientConnReadLoop) processSettings(f *SettingsFrame) error {
	cc := rl.cc
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return f.ForeachSetting(func(s Setting) error {
		switch s.ID {
		case SettingMaxFrameSize:
			cc.maxFrameSize = s.Val
		case SettingMaxConcurrentStreams:
			cc.maxConcurrentStreams = s.Val
		case SettingInitialWindowSize:
			// TODO: error if this is too large.

			// TODO: adjust flow control of still-open
			// frames by the difference of the old initial
			// window size and this one.
			cc.initialWindowSize = s.Val
		default:
			// TODO(bradfitz): handle more settings?
			cc.vlogf("Unhandled Setting: %v", s)
		}
		return nil
	})
}

func (rl *clientConnReadLoop) processWindowUpdate(f *WindowUpdateFrame) error {
	cc := rl.cc
	cs := cc.streamByID(f.StreamID, false)
	if f.StreamID != 0 && cs == nil {
		return nil
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	fl := &cc.flow
	if cs != nil {
		fl = &cs.flow
	}
	if !fl.add(int32(f.Increment)) {
		return ConnectionError(ErrCodeFlowControl)
	}
	cc.cond.Broadcast()
	return nil
}

func (rl *clientConnReadLoop) processResetStream(f *RSTStreamFrame) error {
	cs := rl.cc.streamByID(f.StreamID, true)
	if cs == nil {
		// TODO: return error if server tries to RST_STEAM an idle stream
		return nil
	}
	select {
	case <-cs.peerReset:
		// Already reset.
		// This is the only goroutine
		// which closes this, so there
		// isn't a race.
	default:
		err := StreamError{cs.ID, f.ErrCode}
		cs.resetErr = err
		close(cs.peerReset)
		cs.bufPipe.CloseWithError(err)
		cs.cc.cond.Broadcast() // wake up checkReset via clientStream.awaitFlowControl
	}
	delete(rl.activeRes, cs.ID)
	return nil
}

func (rl *clientConnReadLoop) processPing(f *PingFrame) error {
	if f.IsAck() {
		// 6.7 PING: " An endpoint MUST NOT respond to PING frames
		// containing this flag."
		return nil
	}
	cc := rl.cc
	cc.wmu.Lock()
	defer cc.wmu.Unlock()
	if err := cc.fr.WritePing(true, f.Data); err != nil {
		return err
	}
	return cc.bw.Flush()
}

func (rl *clientConnReadLoop) processPushPromise(f *PushPromiseFrame) error {
	// We told the peer we don't want them.
	// Spec says:
	// "PUSH_PROMISE MUST NOT be sent if the SETTINGS_ENABLE_PUSH
	// setting of the peer endpoint is set to 0. An endpoint that
	// has set this setting and has received acknowledgement MUST
	// treat the receipt of a PUSH_PROMISE frame as a connection
	// error (Section 5.4.1) of type PROTOCOL_ERROR."
	return ConnectionError(ErrCodeProtocol)
}

func (cc *ClientConn) writeStreamReset(streamID uint32, code ErrCode, err error) {
	// TODO: do something with err? send it as a debug frame to the peer?
	// But that's only in GOAWAY. Invent a new frame type? Is there one already?
	cc.wmu.Lock()
	cc.fr.WriteRSTStream(streamID, code)
	cc.wmu.Unlock()
}

// onNewHeaderField runs on the readLoop goroutine whenever a new
// hpack header field is decoded.
func (rl *clientConnReadLoop) onNewHeaderField(f hpack.HeaderField) {
	cc := rl.cc
	if VerboseLogs {
		cc.logf("Header field: %+v", f)
	}
	// TODO: enforce max header list size like server.
	isPseudo := strings.HasPrefix(f.Name, ":")
	if isPseudo {
		if rl.sawRegHeader {
			rl.reqMalformed = errors.New("http2: invalid pseudo header after regular header")
			return
		}
		switch f.Name {
		case ":status":
			code, err := strconv.Atoi(f.Value)
			if err != nil {
				rl.reqMalformed = errors.New("http2: invalid :status")
				return
			}
			rl.nextRes.Status = f.Value + " " + http.StatusText(code)
			rl.nextRes.StatusCode = code
		default:
			// "Endpoints MUST NOT generate pseudo-header
			// fields other than those defined in this
			// document."
			rl.reqMalformed = fmt.Errorf("http2: unknown response pseudo header %q", f.Name)
		}
	} else {
		rl.sawRegHeader = true
		key := http.CanonicalHeaderKey(f.Name)
		if key == "Trailer" {
			t := rl.nextRes.Trailer
			if t == nil {
				t = make(http.Header)
				rl.nextRes.Trailer = t
			}
			foreachHeaderElement(f.Value, func(v string) {
				t[http.CanonicalHeaderKey(v)] = nil
			})
		} else {
			rl.nextRes.Header.Add(key, f.Value)
		}
	}
}

func (cs *clientStream) onNewTrailerField(f hpack.HeaderField) {
	isPseudo := strings.HasPrefix(f.Name, ":")
	if isPseudo {
		// TODO: Bogus. report an error later when we close their body.
		// drop for now.
		return
	}
	key := http.CanonicalHeaderKey(f.Name)
	if _, ok := cs.resTrailer[key]; ok {
		if cs.trailer == nil {
			cs.trailer = make(http.Header)
		}
		const tooBig = 1000 // TODO: arbitrary; use max header list size limits
		if cur := cs.trailer[key]; len(cur) < tooBig {
			cs.trailer[key] = append(cur, f.Value)
		}
	}
}

func (cc *ClientConn) logf(format string, args ...interface{}) {
	cc.t.logf(format, args...)
}

func (cc *ClientConn) vlogf(format string, args ...interface{}) {
	cc.t.vlogf(format, args...)
}

func (t *Transport) vlogf(format string, args ...interface{}) {
	if VerboseLogs {
		t.logf(format, args...)
	}
}

func (t *Transport) logf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

var noBody io.ReadCloser = ioutil.NopCloser(bytes.NewReader(nil))

func strSliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

type erringRoundTripper struct{ err error }

func (rt erringRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, rt.err }

// gzipReader wraps a response body so it can lazily
// call gzip.NewReader on the first call to Read
type gzipReader struct {
	body io.ReadCloser // underlying Response.Body
	zr   io.Reader     // lazily-initialized gzip reader
}

func (gz *gzipReader) Read(p []byte) (n int, err error) {
	if gz.zr == nil {
		gz.zr, err = gzip.NewReader(gz.body)
		if err != nil {
			return 0, err
		}
	}
	return gz.zr.Read(p)
}

func (gz *gzipReader) Close() error {
	return gz.body.Close()
}
