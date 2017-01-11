// Copyright 2012-2016 Apcera Inc. All rights reserved.

package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Type of client connection.
const (
	// CLIENT is an end user.
	CLIENT = iota
	// ROUTER is another router in the cluster.
	ROUTER
)

const (
	// Original Client protocol from 2009.
	// http://nats.io/documentation/internals/nats-protocol/
	ClientProtoZero = iota
	// This signals a client can receive more then the original INFO block.
	// This can be used to update clients on other cluster members, etc.
	ClientProtoInfo
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const (
	// Scratch buffer size for the processMsg() calls.
	msgScratchSize = 512
	msgHeadProto   = "MSG "
)

// For controlling dynamic buffer sizes.
const (
	startBufSize = 512 // For INFO/CONNECT block
	minBufSize   = 128
	maxBufSize   = 65536
)

// Represent client booleans with a bitmask
type clientFlag byte

// Some client state represented as flags
const (
	connectReceived clientFlag = 1 << iota // The CONNECT proto has been received
	firstPongSent                          // The first PONG has been sent
	infoUpdated                            // The server's Info object has changed before first PONG was sent
)

// set the flag (would be equivalent to set the boolean to true)
func (cf *clientFlag) set(c clientFlag) {
	*cf |= c
}

// isSet returns true if the flag is set, false otherwise
func (cf clientFlag) isSet(c clientFlag) bool {
	return cf&c != 0
}

// setIfNotSet will set the flag `c` only if that flag was not already
// set and return true to indicate that the flag has been set. Returns
// false otherwise.
func (cf *clientFlag) setIfNotSet(c clientFlag) bool {
	if *cf&c == 0 {
		*cf |= c
		return true
	}
	return false
}

// clear unset the flag (would be equivalent to set the boolean to false)
func (cf *clientFlag) clear(c clientFlag) {
	*cf &= ^c
}

type client struct {
	// Here first because of use of atomics, and memory alignment.
	stats
	mu    sync.Mutex
	typ   int
	cid   uint64
	lang  string
	opts  clientOpts
	start time.Time
	nc    net.Conn
	mpay  int
	ncs   string
	bw    *bufio.Writer
	srv   *Server
	subs  map[string]*subscription
	perms *permissions
	cache readCache
	pcd   map[*client]struct{}
	atmr  *time.Timer
	ptmr  *time.Timer
	pout  int
	wfc   int
	msgb  [msgScratchSize]byte
	last  time.Time
	parseState

	route *route
	debug bool
	trace bool

	flags clientFlag // Compact booleans into a single field. Size will be increased when needed.
}

type permissions struct {
	sub    *Sublist
	pub    *Sublist
	pcache map[string]bool
}

const (
	maxResultCacheSize = 512
	maxPermCacheSize   = 32
	pruneSize          = 16
)

// Used in readloop to cache hot subject lookups and group statistics.
type readCache struct {
	genid   uint64
	results map[string]*SublistResult
	prand   *rand.Rand
	inMsgs  int
	inBytes int
	subs    int
}

func (c *client) String() (id string) {
	return c.ncs
}

func (c *client) GetOpts() *clientOpts {
	return &c.opts
}

type subscription struct {
	client  *client
	subject []byte
	queue   []byte
	sid     []byte
	nm      int64
	max     int64
}

type clientOpts struct {
	Verbose       bool   `json:"verbose"`
	Pedantic      bool   `json:"pedantic"`
	SslRequired   bool   `json:"ssl_required"`
	Authorization string `json:"auth_token"`
	Username      string `json:"user"`
	Password      string `json:"pass"`
	Name          string `json:"name"`
	Lang          string `json:"lang"`
	Version       string `json:"version"`
	Protocol      int    `json:"protocol"`
}

var defaultOpts = clientOpts{Verbose: true, Pedantic: true}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Lock should be held
func (c *client) initClient() {
	s := c.srv
	c.cid = atomic.AddUint64(&s.gcid, 1)
	c.bw = bufio.NewWriterSize(c.nc, startBufSize)
	c.subs = make(map[string]*subscription)
	c.debug = (atomic.LoadInt32(&debug) != 0)
	c.trace = (atomic.LoadInt32(&trace) != 0)

	// This is a scratch buffer used for processMsg()
	// The msg header starts with "MSG ",
	// in bytes that is [77 83 71 32].
	c.msgb = [msgScratchSize]byte{77, 83, 71, 32}

	// This is to track pending clients that have data to be flushed
	// after we process inbound msgs from our own connection.
	c.pcd = make(map[*client]struct{})

	// snapshot the string version of the connection
	conn := "-"
	if ip, ok := c.nc.(*net.TCPConn); ok {
		addr := ip.RemoteAddr().(*net.TCPAddr)
		conn = fmt.Sprintf("%s:%d", addr.IP, addr.Port)
	}

	switch c.typ {
	case CLIENT:
		c.ncs = fmt.Sprintf("%s - cid:%d", conn, c.cid)
	case ROUTER:
		c.ncs = fmt.Sprintf("%s - rid:%d", conn, c.cid)
	}
}

// RegisterUser allows auth to call back into a new client
// with the authenticated user. This is used to map any permissions
// into the client.
func (c *client) RegisterUser(user *User) {
	if user.Permissions == nil {
		return
	}

	// Process Permissions and map into client connection structures.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Pre-allocate all to simplify checks later.
	c.perms = &permissions{}
	c.perms.sub = NewSublist()
	c.perms.pub = NewSublist()
	c.perms.pcache = make(map[string]bool)

	// Loop over publish permissions
	for _, pubSubject := range user.Permissions.Publish {
		sub := &subscription{subject: []byte(pubSubject)}
		c.perms.pub.Insert(sub)
	}

	// Loop over subscribe permissions
	for _, subSubject := range user.Permissions.Subscribe {
		sub := &subscription{subject: []byte(subSubject)}
		c.perms.sub.Insert(sub)
	}
}

func (c *client) readLoop() {
	// Grab the connection off the client, it will be cleared on a close.
	// We check for that after the loop, but want to avoid a nil dereference
	c.mu.Lock()
	nc := c.nc
	s := c.srv
	defer s.grWG.Done()
	c.mu.Unlock()

	if nc == nil {
		return
	}

	// Start read buffer.
	b := make([]byte, startBufSize)

	for {
		n, err := nc.Read(b)
		if err != nil {
			c.closeConnection()
			return
		}
		// Grab for updates for last activity.
		last := time.Now()

		// Clear inbound stats cache
		c.cache.inMsgs = 0
		c.cache.inBytes = 0
		c.cache.subs = 0

		if err := c.parse(b[:n]); err != nil {
			// handled inline
			if err != ErrMaxPayload && err != ErrAuthorization {
				c.Errorf("Error reading from client: %s", err.Error())
				c.sendErr("Parser Error")
				c.closeConnection()
			}
			return
		}
		// Updates stats for client and server that were collected
		// from parsing through the buffer.
		atomic.AddInt64(&c.inMsgs, int64(c.cache.inMsgs))
		atomic.AddInt64(&c.inBytes, int64(c.cache.inBytes))
		atomic.AddInt64(&s.inMsgs, int64(c.cache.inMsgs))
		atomic.AddInt64(&s.inBytes, int64(c.cache.inBytes))

		// Check pending clients for flush.
		for cp := range c.pcd {
			// Flush those in the set
			cp.mu.Lock()
			if cp.nc != nil {
				// Gather the flush calls that happened before now.
				// This is a signal into us about dynamic buffer allocation tuning.
				wfc := cp.wfc
				cp.wfc = 0

				cp.nc.SetWriteDeadline(time.Now().Add(DEFAULT_FLUSH_DEADLINE))
				err := cp.bw.Flush()
				cp.nc.SetWriteDeadline(time.Time{})
				if err != nil {
					c.Debugf("Error flushing: %v", err)
					cp.mu.Unlock()
					cp.closeConnection()
					cp.mu.Lock()
				} else {
					// Update outbound last activity.
					cp.last = last
					// Check if we should tune the buffer.
					sz := cp.bw.Available()
					// Check for expansion opportunity.
					if wfc > 2 && sz <= maxBufSize/2 {
						cp.bw = bufio.NewWriterSize(cp.nc, sz*2)
					}
					// Check for shrinking opportunity.
					if wfc == 0 && sz >= minBufSize*2 {
						cp.bw = bufio.NewWriterSize(cp.nc, sz/2)
					}
				}
			}
			cp.mu.Unlock()
			delete(c.pcd, cp)
		}
		// Check to see if we got closed, e.g. slow consumer
		c.mu.Lock()
		nc := c.nc
		// Activity based on interest changes or data/msgs.
		if c.cache.inMsgs > 0 || c.cache.subs > 0 {
			c.last = last
		}
		c.mu.Unlock()
		if nc == nil {
			return
		}

		// Update buffer size as/if needed.

		// Grow
		if n == len(b) && len(b) < maxBufSize {
			b = make([]byte, len(b)*2)
		}

		// Shrink, for now don't accelerate, ping/pong will eventually sort it out.
		if n < len(b)/2 && len(b) > minBufSize {
			b = make([]byte, len(b)/2)
		}
	}
}

func (c *client) traceMsg(msg []byte) {
	if !c.trace {
		return
	}
	// FIXME(dlc), allow limits to printable payload
	c.Tracef("->> MSG_PAYLOAD: [%s]", string(msg[:len(msg)-LEN_CR_LF]))
}

func (c *client) traceInOp(op string, arg []byte) {
	c.traceOp("->> %s", op, arg)
}

func (c *client) traceOutOp(op string, arg []byte) {
	c.traceOp("<<- %s", op, arg)
}

func (c *client) traceOp(format, op string, arg []byte) {
	if !c.trace {
		return
	}

	opa := []interface{}{}
	if op != "" {
		opa = append(opa, op)
	}
	if arg != nil {
		opa = append(opa, string(arg))
	}
	c.Tracef(format, opa)
}

// Process the information messages from Clients and other Routes.
func (c *client) processInfo(arg []byte) error {
	info := Info{}
	if err := json.Unmarshal(arg, &info); err != nil {
		return err
	}
	if c.typ == ROUTER {
		c.processRouteInfo(&info)
	}
	return nil
}

func (c *client) processErr(errStr string) {
	switch c.typ {
	case CLIENT:
		c.Errorf("Client Error %s", errStr)
	case ROUTER:
		c.Errorf("Route Error %s", errStr)
	}
	c.closeConnection()
}

func (c *client) processConnect(arg []byte) error {
	c.traceInOp("CONNECT", arg)

	c.mu.Lock()
	// If we can't stop the timer because the callback is in progress...
	if !c.clearAuthTimer() {
		// wait for it to finish and handle sending the failure back to
		// the client.
		for c.nc != nil {
			c.mu.Unlock()
			time.Sleep(25 * time.Millisecond)
			c.mu.Lock()
		}
		c.mu.Unlock()
		return nil
	}
	c.last = time.Now()
	typ := c.typ
	r := c.route
	srv := c.srv
	// Moved unmarshalling of clients' Options under the lock.
	// The client has already been added to the server map, so it is possible
	// that other routines lookup the client, and access its options under
	// the client's lock, so unmarshalling the options outside of the lock
	// would cause data RACEs.
	if err := json.Unmarshal(arg, &c.opts); err != nil {
		c.mu.Unlock()
		return err
	}
	// Indicate that the CONNECT protocol has been received, and that the
	// server now knows which protocol this client supports.
	c.flags.set(connectReceived)
	// Capture these under lock
	proto := c.opts.Protocol
	verbose := c.opts.Verbose
	c.mu.Unlock()

	if srv != nil {
		// As soon as c.opts is unmarshalled and if the proto is at
		// least ClientProtoInfo, we need to increment the following counter.
		// This is decremented when client is removed from the server's
		// clients map.
		if proto >= ClientProtoInfo {
			srv.mu.Lock()
			srv.cproto++
			srv.mu.Unlock()
		}

		// Check for Auth
		if ok := srv.checkAuth(c); !ok {
			c.authViolation()
			return ErrAuthorization
		}
	}

	// Check client protocol request if it exists.
	if typ == CLIENT && (proto < ClientProtoZero || proto > ClientProtoInfo) {
		return ErrBadClientProtocol
	}

	// Grab connection name of remote route.
	if typ == ROUTER && r != nil {
		c.mu.Lock()
		c.route.remoteID = c.opts.Name
		c.mu.Unlock()
	}

	if verbose {
		c.sendOK()
	}
	return nil
}

func (c *client) authTimeout() {
	c.sendErr(ErrAuthTimeout.Error())
	c.Debugf("Authorization Timeout")
	c.closeConnection()
}

func (c *client) authViolation() {
	if c.srv != nil && c.srv.opts.Users != nil {
		c.Errorf("%s - User %q",
			ErrAuthorization.Error(),
			c.opts.Username)
	} else {
		c.Errorf(ErrAuthorization.Error())
	}
	c.sendErr("Authorization Violation")
	c.closeConnection()
}

func (c *client) maxConnExceeded() {
	c.Errorf(ErrTooManyConnections.Error())
	c.sendErr(ErrTooManyConnections.Error())
	c.closeConnection()
}

func (c *client) maxPayloadViolation(sz int) {
	c.Errorf("%s: %d vs %d", ErrMaxPayload.Error(), sz, c.mpay)
	c.sendErr("Maximum Payload Violation")
	c.closeConnection()
}

// Assume the lock is held upon entry.
func (c *client) sendProto(info []byte, doFlush bool) error {
	var err error
	if c.bw != nil && c.nc != nil {
		deadlineSet := false
		if doFlush || c.bw.Available() < len(info) {
			c.nc.SetWriteDeadline(time.Now().Add(DEFAULT_FLUSH_DEADLINE))
			deadlineSet = true
		}
		_, err = c.bw.Write(info)
		if err == nil && doFlush {
			err = c.bw.Flush()
		}
		if deadlineSet {
			c.nc.SetWriteDeadline(time.Time{})
		}
	}
	return err
}

// Assume the lock is held upon entry.
func (c *client) sendInfo(info []byte) {
	c.sendProto(info, true)
}

func (c *client) sendErr(err string) {
	c.mu.Lock()
	c.traceOutOp("-ERR", []byte(err))
	c.sendProto([]byte(fmt.Sprintf("-ERR '%s'\r\n", err)), true)
	c.mu.Unlock()
}

func (c *client) sendOK() {
	c.mu.Lock()
	c.traceOutOp("OK", nil)
	// Can not autoflush this one, needs to be async.
	c.sendProto([]byte("+OK\r\n"), false)
	c.pcd[c] = needFlush
	c.mu.Unlock()
}

func (c *client) processPing() {
	c.mu.Lock()
	c.traceInOp("PING", nil)
	if c.nc == nil {
		c.mu.Unlock()
		return
	}
	c.traceOutOp("PONG", nil)
	err := c.sendProto([]byte("PONG\r\n"), true)
	if err != nil {
		c.clearConnection()
		c.Debugf("Error on Flush, error %s", err.Error())
	}
	srv := c.srv
	sendUpdateINFO := false
	// Check if this is the first PONG, if so...
	if c.flags.setIfNotSet(firstPongSent) {
		// Check if server should send an async INFO protocol to the client
		if c.opts.Protocol >= ClientProtoInfo &&
			srv != nil && c.flags.isSet(infoUpdated) {
			sendUpdateINFO = true
		}
		// We can now clear the flag
		c.flags.clear(infoUpdated)
	}
	c.mu.Unlock()

	// Some clients send an initial PING as part of the synchronous connect process.
	// They can't be receiving anything until the first PONG is received.
	// So we delay the possible updated INFO after this point.
	if sendUpdateINFO {
		srv.mu.Lock()
		// Use the cached protocol
		proto := srv.infoJSON
		srv.mu.Unlock()

		c.mu.Lock()
		c.sendInfo(proto)
		c.mu.Unlock()
	}
}

func (c *client) processPong() {
	c.traceInOp("PONG", nil)
	c.mu.Lock()
	c.pout = 0
	c.mu.Unlock()
}

func (c *client) processMsgArgs(arg []byte) error {
	if c.trace {
		c.traceInOp("MSG", arg)
	}

	// Unroll splitArgs to avoid runtime/heap issues
	a := [MAX_MSG_ARGS][]byte{}
	args := a[:0]
	start := -1
	for i, b := range arg {
		switch b {
		case ' ', '\t', '\r', '\n':
			if start >= 0 {
				args = append(args, arg[start:i])
				start = -1
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		args = append(args, arg[start:])
	}

	switch len(args) {
	case 3:
		c.pa.reply = nil
		c.pa.szb = args[2]
		c.pa.size = parseSize(args[2])
	case 4:
		c.pa.reply = args[2]
		c.pa.szb = args[3]
		c.pa.size = parseSize(args[3])
	default:
		return fmt.Errorf("processMsgArgs Parse Error: '%s'", arg)
	}
	if c.pa.size < 0 {
		return fmt.Errorf("processMsgArgs Bad or Missing Size: '%s'", arg)
	}

	// Common ones processed after check for arg length
	c.pa.subject = args[0]
	c.pa.sid = args[1]

	return nil
}

func (c *client) processPub(arg []byte) error {
	if c.trace {
		c.traceInOp("PUB", arg)
	}

	// Unroll splitArgs to avoid runtime/heap issues
	a := [MAX_PUB_ARGS][]byte{}
	args := a[:0]
	start := -1
	for i, b := range arg {
		switch b {
		case ' ', '\t', '\r', '\n':
			if start >= 0 {
				args = append(args, arg[start:i])
				start = -1
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		args = append(args, arg[start:])
	}

	switch len(args) {
	case 2:
		c.pa.subject = args[0]
		c.pa.reply = nil
		c.pa.size = parseSize(args[1])
		c.pa.szb = args[1]
	case 3:
		c.pa.subject = args[0]
		c.pa.reply = args[1]
		c.pa.size = parseSize(args[2])
		c.pa.szb = args[2]
	default:
		return fmt.Errorf("processPub Parse Error: '%s'", arg)
	}
	if c.pa.size < 0 {
		return fmt.Errorf("processPub Bad or Missing Size: '%s'", arg)
	}
	if c.mpay > 0 && c.pa.size > c.mpay {
		c.maxPayloadViolation(c.pa.size)
		return ErrMaxPayload
	}

	if c.opts.Pedantic && !IsValidLiteralSubject(string(c.pa.subject)) {
		c.sendErr("Invalid Subject")
	}
	return nil
}

func splitArg(arg []byte) [][]byte {
	a := [MAX_MSG_ARGS][]byte{}
	args := a[:0]
	start := -1
	for i, b := range arg {
		switch b {
		case ' ', '\t', '\r', '\n':
			if start >= 0 {
				args = append(args, arg[start:i])
				start = -1
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		args = append(args, arg[start:])
	}
	return args
}

func (c *client) processSub(argo []byte) (err error) {
	c.traceInOp("SUB", argo)

	// Indicate activity.
	c.cache.subs += 1

	// Copy so we do not reference a potentially large buffer
	arg := make([]byte, len(argo))
	copy(arg, argo)
	args := splitArg(arg)
	sub := &subscription{client: c}
	switch len(args) {
	case 2:
		sub.subject = args[0]
		sub.queue = nil
		sub.sid = args[1]
	case 3:
		sub.subject = args[0]
		sub.queue = args[1]
		sub.sid = args[2]
	default:
		return fmt.Errorf("processSub Parse Error: '%s'", arg)
	}

	shouldForward := false

	c.mu.Lock()
	if c.nc == nil {
		c.mu.Unlock()
		return nil
	}

	// Check permissions if applicable.
	if c.perms != nil {
		r := c.perms.sub.Match(string(sub.subject))
		if len(r.psubs) == 0 {
			c.mu.Unlock()
			c.sendErr(fmt.Sprintf("Permissions Violation for Subscription to %q", sub.subject))
			c.Errorf("Subscription Violation - User %q, Subject %q", c.opts.Username, sub.subject)
			return nil
		}
	}

	// We can have two SUB protocols coming from a route due to some
	// race conditions. We should make sure that we process only one.
	sid := string(sub.sid)
	if c.subs[sid] == nil {
		c.subs[sid] = sub
		if c.srv != nil {
			err = c.srv.sl.Insert(sub)
			if err != nil {
				delete(c.subs, sid)
			} else {
				shouldForward = c.typ != ROUTER
			}
		}
	}
	c.mu.Unlock()
	if err != nil {
		c.sendErr("Invalid Subject")
		return nil
	} else if c.opts.Verbose {
		c.sendOK()
	}
	if shouldForward {
		c.srv.broadcastSubscribe(sub)
	}

	return nil
}

func (c *client) unsubscribe(sub *subscription) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sub.max > 0 && sub.nm < sub.max {
		c.Debugf(
			"Deferring actual UNSUB(%s): %d max, %d received\n",
			string(sub.subject), sub.max, sub.nm)
		return
	}
	c.traceOp("<-> %s", "DELSUB", sub.sid)
	delete(c.subs, string(sub.sid))
	if c.srv != nil {
		c.srv.sl.Remove(sub)
	}
}

func (c *client) processUnsub(arg []byte) error {
	c.traceInOp("UNSUB", arg)
	args := splitArg(arg)
	var sid []byte
	max := -1

	switch len(args) {
	case 1:
		sid = args[0]
	case 2:
		sid = args[0]
		max = parseSize(args[1])
	default:
		return fmt.Errorf("processUnsub Parse Error: '%s'", arg)
	}

	// Indicate activity.
	c.cache.subs += 1

	var sub *subscription

	unsub := false
	shouldForward := false
	ok := false

	c.mu.Lock()
	if sub, ok = c.subs[string(sid)]; ok {
		if max > 0 {
			sub.max = int64(max)
		} else {
			// Clear it here to override
			sub.max = 0
		}
		unsub = true
		shouldForward = c.typ != ROUTER && c.srv != nil
	}
	c.mu.Unlock()

	if unsub {
		c.unsubscribe(sub)
	}
	if shouldForward {
		c.srv.broadcastUnSubscribe(sub)
	}
	if c.opts.Verbose {
		c.sendOK()
	}

	return nil
}

func (c *client) msgHeader(mh []byte, sub *subscription) []byte {
	mh = append(mh, sub.sid...)
	mh = append(mh, ' ')
	if c.pa.reply != nil {
		mh = append(mh, c.pa.reply...)
		mh = append(mh, ' ')
	}
	mh = append(mh, c.pa.szb...)
	mh = append(mh, "\r\n"...)
	return mh
}

// Used to treat maps as efficient set
var needFlush = struct{}{}
var routeSeen = struct{}{}

func (c *client) deliverMsg(sub *subscription, mh, msg []byte) {
	if sub.client == nil {
		return
	}
	client := sub.client
	client.mu.Lock()
	sub.nm++
	// Check if we should auto-unsubscribe.
	if sub.max > 0 {
		// For routing..
		shouldForward := client.typ != ROUTER && client.srv != nil
		// If we are at the exact number, unsubscribe but
		// still process the message in hand, otherwise
		// unsubscribe and drop message on the floor.
		if sub.nm == sub.max {
			c.Debugf("Auto-unsubscribe limit of %d reached for sid '%s'\n", sub.max, string(sub.sid))
			// Due to defer, reverse the code order so that execution
			// is consistent with other cases where we unsubscribe.
			if shouldForward {
				defer client.srv.broadcastUnSubscribe(sub)
			}
			defer client.unsubscribe(sub)
		} else if sub.nm > sub.max {
			c.Debugf("Auto-unsubscribe limit [%d] exceeded\n", sub.max)
			client.mu.Unlock()
			client.unsubscribe(sub)
			if shouldForward {
				client.srv.broadcastUnSubscribe(sub)
			}
			return
		}
	}

	if client.nc == nil {
		client.mu.Unlock()
		return
	}

	// Update statistics

	// The msg includes the CR_LF, so pull back out for accounting.
	msgSize := int64(len(msg) - LEN_CR_LF)

	// No atomic needed since accessed under client lock.
	// Monitor is reading those also under client's lock.
	client.outMsgs++
	client.outBytes += msgSize

	atomic.AddInt64(&c.srv.outMsgs, 1)
	atomic.AddInt64(&c.srv.outBytes, msgSize)

	// Check to see if our writes will cause a flush
	// in the underlying bufio. If so limit time we
	// will wait for flush to complete.

	deadlineSet := false
	if client.bw.Available() < (len(mh) + len(msg)) {
		client.wfc += 1
		client.nc.SetWriteDeadline(time.Now().Add(DEFAULT_FLUSH_DEADLINE))
		deadlineSet = true
	}

	// Deliver to the client.
	_, err := client.bw.Write(mh)
	if err != nil {
		goto writeErr
	}

	_, err = client.bw.Write(msg)
	if err != nil {
		goto writeErr
	}

	if c.trace {
		client.traceOutOp(string(mh[:len(mh)-LEN_CR_LF]), nil)
	}

	// TODO(dlc) - Do we need this or can we just call always?
	if deadlineSet {
		client.nc.SetWriteDeadline(time.Time{})
	}

	client.mu.Unlock()
	c.pcd[client] = needFlush
	return

writeErr:
	if deadlineSet {
		client.nc.SetWriteDeadline(time.Time{})
	}
	client.mu.Unlock()

	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		atomic.AddInt64(&client.srv.slowConsumers, 1)
		client.Noticef("Slow Consumer Detected")
		client.closeConnection()
	} else {
		c.Debugf("Error writing msg: %v", err)
	}
}

// processMsg is called to process an inbound msg from a client.
func (c *client) processMsg(msg []byte) {
	// Snapshot server.
	srv := c.srv

	// Update statistics
	// The msg includes the CR_LF, so pull back out for accounting.
	c.cache.inMsgs += 1
	c.cache.inBytes += len(msg) - LEN_CR_LF

	if c.trace {
		c.traceMsg(msg)
	}

	// defintely

	// Disallow publish to _SYS.>, these are reserved for internals.
	if c.pa.subject[0] == '_' && len(c.pa.subject) > 4 &&
		c.pa.subject[1] == 'S' && c.pa.subject[2] == 'Y' &&
		c.pa.subject[3] == 'S' && c.pa.subject[4] == '.' {
		c.pubPermissionViolation(c.pa.subject)
		return
	}

	// Check if published subject is allowed if we have permissions in place.
	if c.perms != nil {
		allowed, ok := c.perms.pcache[string(c.pa.subject)]
		if ok && !allowed {
			c.pubPermissionViolation(c.pa.subject)
			return
		}
		if !ok {
			r := c.perms.pub.Match(string(c.pa.subject))
			notAllowed := len(r.psubs) == 0
			if notAllowed {
				c.pubPermissionViolation(c.pa.subject)
				c.perms.pcache[string(c.pa.subject)] = false
			} else {
				c.perms.pcache[string(c.pa.subject)] = true
			}
			// Prune if needed.
			if len(c.perms.pcache) > maxPermCacheSize {
				// Prune the permissions cache. Keeps us from unbounded growth.
				r := 0
				for subject := range c.perms.pcache {
					delete(c.cache.results, subject)
					r++
					if r > pruneSize {
						break
					}
				}
			}
			// Return here to allow the pruning code to run if needed.
			if notAllowed {
				return
			}
		}
	}

	if c.opts.Verbose {
		c.sendOK()
	}

	// Mostly under testing scenarios.
	if srv == nil {
		return
	}

	var r *SublistResult
	var ok bool

	genid := atomic.LoadUint64(&srv.sl.genid)

	if genid == c.cache.genid && c.cache.results != nil {
		r, ok = c.cache.results[string(c.pa.subject)]
	} else {
		// reset
		c.cache.results = make(map[string]*SublistResult)
		c.cache.genid = genid
	}

	if !ok {
		subject := string(c.pa.subject)
		r = srv.sl.Match(subject)
		c.cache.results[subject] = r
		if len(c.cache.results) > maxResultCacheSize {
			// Prune the results cache. Keeps us from unbounded growth.
			r := 0
			for subject := range c.cache.results {
				delete(c.cache.results, subject)
				r++
				if r > pruneSize {
					break
				}
			}
		}
	}

	// Check for no interest, short circuit if so.
	if len(r.psubs) == 0 && len(r.qsubs) == 0 {
		return
	}

	// Check for pedantic and bad subject.
	if c.opts.Pedantic && !IsValidLiteralSubject(string(c.pa.subject)) {
		return
	}

	// Scratch buffer..
	msgh := c.msgb[:len(msgHeadProto)]

	// msg header
	msgh = append(msgh, c.pa.subject...)
	msgh = append(msgh, ' ')
	si := len(msgh)

	isRoute := c.typ == ROUTER

	// If we are a route and we have a queue subscription, deliver direct
	// since they are sent direct via L2 semantics. If the match is a queue
	// subscription, we will return from here regardless if we find a sub.
	if isRoute {
		if sub, ok := srv.routeSidQueueSubscriber(c.pa.sid); ok {
			if sub != nil {
				mh := c.msgHeader(msgh[:si], sub)
				c.deliverMsg(sub, mh, msg)
			}
			return
		}
	}

	// Used to only send normal subscriptions once across a given route.
	var rmap map[string]struct{}

	// Loop over all normal subscriptions that match.

	for _, sub := range r.psubs {
		// Check if this is a send to a ROUTER, make sure we only send it
		// once. The other side will handle the appropriate re-processing
		// and fan-out. Also enforce 1-Hop semantics, so no routing to another.
		if sub.client.typ == ROUTER {
			// Skip if sourced from a ROUTER and going to another ROUTER.
			// This is 1-Hop semantics for ROUTERs.
			if isRoute {
				continue
			}
			// Check to see if we have already sent it here.
			if rmap == nil {
				rmap = make(map[string]struct{}, srv.numRoutes())
			}
			sub.client.mu.Lock()
			if sub.client.nc == nil || sub.client.route == nil ||
				sub.client.route.remoteID == "" {
				c.Debugf("Bad or Missing ROUTER Identity, not processing msg")
				sub.client.mu.Unlock()
				continue
			}
			if _, ok := rmap[sub.client.route.remoteID]; ok {
				c.Debugf("Ignoring route, already processed")
				sub.client.mu.Unlock()
				continue
			}
			rmap[sub.client.route.remoteID] = routeSeen
			sub.client.mu.Unlock()
		}
		// Normal delivery
		mh := c.msgHeader(msgh[:si], sub)
		c.deliverMsg(sub, mh, msg)
	}

	// Now process any queue subs we have if not a route
	if !isRoute {
		// Check to see if we have our own rand yet. Global rand
		// has contention with lots of clients, etc.
		if c.cache.prand == nil {
			c.cache.prand = rand.New(rand.NewSource(time.Now().UnixNano()))
		}
		// Process queue subs
		for i := 0; i < len(r.qsubs); i++ {
			qsubs := r.qsubs[i]
			index := c.cache.prand.Intn(len(qsubs))
			sub := qsubs[index]
			if sub != nil {
				mh := c.msgHeader(msgh[:si], sub)
				c.deliverMsg(sub, mh, msg)
			}
		}
	}
}

func (c *client) pubPermissionViolation(subject []byte) {
	c.sendErr(fmt.Sprintf("Permissions Violation for Publish to %q", subject))
	c.Errorf("Publish Violation - User %q, Subject %q", c.opts.Username, subject)
}

func (c *client) processPingTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ptmr = nil
	// Check if we are ready yet..
	if _, ok := c.nc.(*net.TCPConn); !ok {
		return
	}

	c.Debugf("%s Ping Timer", c.typeString())

	// Check for violation
	c.pout++
	if c.pout > c.srv.opts.MaxPingsOut {
		c.Debugf("Stale Client Connection - Closing")
		c.sendProto([]byte(fmt.Sprintf("-ERR '%s'\r\n", "Stale Connection")), true)
		c.clearConnection()
		return
	}

	c.traceOutOp("PING", nil)

	// Send PING
	err := c.sendProto([]byte("PING\r\n"), true)
	if err != nil {
		c.Debugf("Error on Client Ping Flush, error %s", err)
		c.clearConnection()
	} else {
		// Reset to fire again if all OK.
		c.setPingTimer()
	}
}

func (c *client) setPingTimer() {
	if c.srv == nil {
		return
	}
	d := c.srv.opts.PingInterval
	c.ptmr = time.AfterFunc(d, c.processPingTimer)
}

// Lock should be held
func (c *client) clearPingTimer() {
	if c.ptmr == nil {
		return
	}
	c.ptmr.Stop()
	c.ptmr = nil
}

// Lock should be held
func (c *client) setAuthTimer(d time.Duration) {
	c.atmr = time.AfterFunc(d, func() { c.authTimeout() })
}

// Lock should be held
func (c *client) clearAuthTimer() bool {
	if c.atmr == nil {
		return true
	}
	stopped := c.atmr.Stop()
	c.atmr = nil
	return stopped
}

func (c *client) isAuthTimerSet() bool {
	c.mu.Lock()
	isSet := c.atmr != nil
	c.mu.Unlock()
	return isSet
}

// Lock should be held
func (c *client) clearConnection() {
	if c.nc == nil {
		return
	}
	// With TLS, Close() is sending an alert (that is doing a write).
	// Need to set a deadline otherwise the server could block there
	// if the peer is not reading from socket.
	c.nc.SetWriteDeadline(time.Now().Add(DEFAULT_FLUSH_DEADLINE))
	if c.bw != nil {
		c.bw.Flush()
	}
	c.nc.Close()
	c.nc.SetWriteDeadline(time.Time{})
}

func (c *client) typeString() string {
	switch c.typ {
	case CLIENT:
		return "Client"
	case ROUTER:
		return "Router"
	}
	return "Unknown Type"
}

func (c *client) closeConnection() {
	c.mu.Lock()
	if c.nc == nil {
		c.mu.Unlock()
		return
	}

	c.Debugf("%s connection closed", c.typeString())

	c.clearAuthTimer()
	c.clearPingTimer()
	c.clearConnection()
	c.nc = nil

	// Snapshot for use.
	subs := make([]*subscription, 0, len(c.subs))
	for _, sub := range c.subs {
		subs = append(subs, sub)
	}
	srv := c.srv

	retryImplicit := false
	if c.route != nil {
		retryImplicit = c.route.retry
	}

	c.mu.Unlock()

	if srv != nil {
		// Unregister
		srv.removeClient(c)

		// Remove clients subscriptions.
		for _, sub := range subs {
			srv.sl.Remove(sub)
			// Forward on unsubscribes if we are not
			// a router ourselves.
			if c.typ != ROUTER {
				srv.broadcastUnSubscribe(sub)
			}
		}
	}

	// Check for a solicited route. If it was, start up a reconnect unless
	// we are already connected to the other end.
	if c.isSolicitedRoute() || retryImplicit {
		// Capture these under lock
		c.mu.Lock()
		rid := c.route.remoteID
		rtype := c.route.routeType
		rurl := c.route.url
		c.mu.Unlock()

		srv.mu.Lock()
		defer srv.mu.Unlock()

		// It is possible that the server is being shutdown.
		// If so, don't try to reconnect
		if !srv.running {
			return
		}

		if rid != "" && srv.remotes[rid] != nil {
			Debugf("Not attempting reconnect for solicited route, already connected to \"%s\"", rid)
			return
		} else if rid == srv.info.ID {
			Debugf("Detected route to self, ignoring \"%s\"", rurl)
			return
		} else if rtype != Implicit || retryImplicit {
			Debugf("Attempting reconnect for solicited route \"%s\"", rurl)
			// Keep track of this go-routine so we can wait for it on
			// server shutdown.
			srv.startGoRoutine(func() { srv.reConnectToRoute(rurl, rtype) })
		}
	}
}

// Logging functionality scoped to a client or route.

func (c *client) Errorf(format string, v ...interface{}) {
	format = fmt.Sprintf("%s - %s", c, format)
	Errorf(format, v...)
}

func (c *client) Debugf(format string, v ...interface{}) {
	format = fmt.Sprintf("%s - %s", c, format)
	Debugf(format, v...)
}

func (c *client) Noticef(format string, v ...interface{}) {
	format = fmt.Sprintf("%s - %s", c, format)
	Noticef(format, v...)
}

func (c *client) Tracef(format string, v ...interface{}) {
	format = fmt.Sprintf("%s - %s", c, format)
	Tracef(format, v...)
}
