// Copyright 2012-2016 Apcera Inc. All rights reserved.

package test

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/nats-io/gnatsd/auth"
	"github.com/nats-io/gnatsd/server"
)

const natsServerExe = "../gnatsd"

type natsServer struct {
	args []string
	cmd  *exec.Cmd
}

// So we can pass tests and benchmarks..
type tLogger interface {
	Fatalf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// DefaultTestOptions are default options for the unit tests.
var DefaultTestOptions = server.Options{
	Host:           "localhost",
	Port:           4222,
	NoLog:          true,
	NoSigs:         true,
	MaxControlLine: 256,
}

// RunDefaultServer starts a new Go routine based server using the default options
func RunDefaultServer() *server.Server {
	return RunServer(&DefaultTestOptions)
}

// RunServer starts a new Go routine based server
func RunServer(opts *server.Options) *server.Server {
	return RunServerWithAuth(opts, nil)
}

// LoadConfig loads a configuration from a filename
func LoadConfig(configFile string) (opts *server.Options) {
	opts, err := server.ProcessConfigFile(configFile)
	if err != nil {
		panic(fmt.Sprintf("Error processing configuration file: %v", err))
	}
	opts.NoSigs, opts.NoLog = true, true
	return
}

// RunServerWithConfig starts a new Go routine based server with a configuration file.
func RunServerWithConfig(configFile string) (srv *server.Server, opts *server.Options) {
	opts = LoadConfig(configFile)

	// Check for auth
	var a server.Auth
	if opts.Authorization != "" {
		a = &auth.Token{Token: opts.Authorization}
	}
	if opts.Username != "" {
		a = &auth.Plain{Username: opts.Username, Password: opts.Password}
	}
	if opts.Users != nil {
		a = auth.NewMultiUser(opts.Users)
	}
	srv = RunServerWithAuth(opts, a)
	return
}

// RunServerWithAuth starts a new Go routine based server with auth
func RunServerWithAuth(opts *server.Options, auth server.Auth) *server.Server {
	if opts == nil {
		opts = &DefaultTestOptions
	}
	s := server.New(opts)
	if s == nil {
		panic("No NATS Server object returned.")
	}

	if auth != nil {
		s.SetClientAuthMethod(auth)
	}

	// Run server in Go routine.
	go s.Start()

	// Wait for accept loop(s) to be started
	if !s.ReadyForConnections(10 * time.Second) {
		panic("Unable to start NATS Server in Go Routine")
	}
	return s
}

func stackFatalf(t tLogger, f string, args ...interface{}) {
	lines := make([]string, 0, 32)
	msg := fmt.Sprintf(f, args...)
	lines = append(lines, msg)

	// Ignore ourselves
	_, testFile, _, _ := runtime.Caller(0)

	// Generate the Stack of callers:
	for i := 0; true; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		if file == testFile {
			continue
		}
		msg := fmt.Sprintf("%d - %s:%d", i, file, line)
		lines = append(lines, msg)
	}

	t.Fatalf("%s", strings.Join(lines, "\n"))
}

func acceptRouteConn(t tLogger, host string, timeout time.Duration) net.Conn {
	l, e := net.Listen("tcp", host)
	if e != nil {
		stackFatalf(t, "Error listening for route connection on %v: %v", host, e)
	}
	defer l.Close()

	tl := l.(*net.TCPListener)
	tl.SetDeadline(time.Now().Add(timeout))
	conn, err := l.Accept()
	tl.SetDeadline(time.Time{})

	if err != nil {
		stackFatalf(t, "Did not receive a route connection request: %v", err)
	}
	return conn
}

func createRouteConn(t tLogger, host string, port int) net.Conn {
	return createClientConn(t, host, port)
}

func createClientConn(t tLogger, host string, port int) net.Conn {
	addr := fmt.Sprintf("%s:%d", host, port)
	c, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		stackFatalf(t, "Could not connect to server: %v\n", err)
	}
	return c
}

func checkSocket(t tLogger, addr string, wait time.Duration) {
	end := time.Now().Add(wait)
	for time.Now().Before(end) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			// Retry after 50ms
			time.Sleep(50 * time.Millisecond)
			continue
		}
		conn.Close()
		// Wait a bit to give a chance to the server to remove this
		// "client" from its state, which may otherwise interfere with
		// some tests.
		time.Sleep(25 * time.Millisecond)
		return
	}
	// We have failed to bind the socket in the time allowed.
	t.Fatalf("Failed to connect to the socket: %q", addr)
}

func checkInfoMsg(t tLogger, c net.Conn) server.Info {
	buf := expectResult(t, c, infoRe)
	js := infoRe.FindAllSubmatch(buf, 1)[0][1]
	var sinfo server.Info
	err := json.Unmarshal(js, &sinfo)
	if err != nil {
		stackFatalf(t, "Could not unmarshal INFO json: %v\n", err)
	}
	return sinfo
}

func doConnect(t tLogger, c net.Conn, verbose, pedantic, ssl bool) {
	checkInfoMsg(t, c)
	cs := fmt.Sprintf("CONNECT {\"verbose\":%v,\"pedantic\":%v,\"ssl_required\":%v}\r\n", verbose, pedantic, ssl)
	sendProto(t, c, cs)
}

func doDefaultConnect(t tLogger, c net.Conn) {
	// Basic Connect
	doConnect(t, c, false, false, false)
}

const connectProto = "CONNECT {\"verbose\":false,\"user\":\"%s\",\"pass\":\"%s\",\"name\":\"%s\"}\r\n"

func doRouteAuthConnect(t tLogger, c net.Conn, user, pass, id string) {
	cs := fmt.Sprintf(connectProto, user, pass, id)
	sendProto(t, c, cs)
}

func setupRouteEx(t tLogger, c net.Conn, opts *server.Options, id string) (sendFun, expectFun) {
	user := opts.Cluster.Username
	pass := opts.Cluster.Password
	doRouteAuthConnect(t, c, user, pass, id)
	return sendCommand(t, c), expectCommand(t, c)
}

func setupRoute(t tLogger, c net.Conn, opts *server.Options) (sendFun, expectFun) {
	u := make([]byte, 16)
	io.ReadFull(rand.Reader, u)
	id := fmt.Sprintf("ROUTER:%s", hex.EncodeToString(u))
	return setupRouteEx(t, c, opts, id)
}

func setupConn(t tLogger, c net.Conn) (sendFun, expectFun) {
	doDefaultConnect(t, c)
	return sendCommand(t, c), expectCommand(t, c)
}

func setupConnWithProto(t tLogger, c net.Conn, proto int) (sendFun, expectFun) {
	checkInfoMsg(t, c)
	cs := fmt.Sprintf("CONNECT {\"verbose\":%v,\"pedantic\":%v,\"ssl_required\":%v,\"protocol\":%d}\r\n", false, false, false, proto)
	sendProto(t, c, cs)
	return sendCommand(t, c), expectCommand(t, c)
}

type sendFun func(string)
type expectFun func(*regexp.Regexp) []byte

// Closure version for easier reading
func sendCommand(t tLogger, c net.Conn) sendFun {
	return func(op string) {
		sendProto(t, c, op)
	}
}

// Closure version for easier reading
func expectCommand(t tLogger, c net.Conn) expectFun {
	return func(re *regexp.Regexp) []byte {
		return expectResult(t, c, re)
	}
}

// Send the protocol command to the server.
func sendProto(t tLogger, c net.Conn, op string) {
	n, err := c.Write([]byte(op))
	if err != nil {
		stackFatalf(t, "Error writing command to conn: %v\n", err)
	}
	if n != len(op) {
		stackFatalf(t, "Partial write: %d vs %d\n", n, len(op))
	}
}

var (
	infoRe       = regexp.MustCompile(`INFO\s+([^\r\n]+)\r\n`)
	pingRe       = regexp.MustCompile(`PING\r\n`)
	pongRe       = regexp.MustCompile(`PONG\r\n`)
	msgRe        = regexp.MustCompile(`(?:(?:MSG\s+([^\s]+)\s+([^\s]+)\s+(([^\s]+)[^\S\r\n]+)?(\d+)\s*\r\n([^\\r\\n]*?)\r\n)+?)`)
	okRe         = regexp.MustCompile(`\A\+OK\r\n`)
	errRe        = regexp.MustCompile(`\A\-ERR\s+([^\r\n]+)\r\n`)
	subRe        = regexp.MustCompile(`SUB\s+([^\s]+)((\s+)([^\s]+))?\s+([^\s]+)\r\n`)
	unsubRe      = regexp.MustCompile(`UNSUB\s+([^\s]+)(\s+(\d+))?\r\n`)
	unsubmaxRe   = regexp.MustCompile(`UNSUB\s+([^\s]+)(\s+(\d+))\r\n`)
	unsubnomaxRe = regexp.MustCompile(`UNSUB\s+([^\s]+)\r\n`)
	connectRe    = regexp.MustCompile(`CONNECT\s+([^\r\n]+)\r\n`)
)

const (
	subIndex   = 1
	sidIndex   = 2
	replyIndex = 4
	lenIndex   = 5
	msgIndex   = 6
)

// Test result from server against regexp
func expectResult(t tLogger, c net.Conn, re *regexp.Regexp) []byte {
	expBuf := make([]byte, 32768)
	// Wait for commands to be processed and results queued for read
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := c.Read(expBuf)
	c.SetReadDeadline(time.Time{})

	if n <= 0 && err != nil {
		stackFatalf(t, "Error reading from conn: %v\n", err)
	}
	buf := expBuf[:n]

	if !re.Match(buf) {
		stackFatalf(t, "Response did not match expected: \n\tReceived:'%q'\n\tExpected:'%s'\n", buf, re)
	}
	return buf
}

func expectNothing(t tLogger, c net.Conn) {
	expBuf := make([]byte, 32)
	c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	n, err := c.Read(expBuf)
	c.SetReadDeadline(time.Time{})
	if err == nil && n > 0 {
		stackFatalf(t, "Expected nothing, received: '%q'\n", expBuf[:n])
	}
}

// This will check that we got what we expected.
func checkMsg(t tLogger, m [][]byte, subject, sid, reply, len, msg string) {
	if string(m[subIndex]) != subject {
		stackFatalf(t, "Did not get correct subject: expected '%s' got '%s'\n", subject, m[subIndex])
	}
	if sid != "" && string(m[sidIndex]) != sid {
		stackFatalf(t, "Did not get correct sid: expected '%s' got '%s'\n", sid, m[sidIndex])
	}
	if string(m[replyIndex]) != reply {
		stackFatalf(t, "Did not get correct reply: expected '%s' got '%s'\n", reply, m[replyIndex])
	}
	if string(m[lenIndex]) != len {
		stackFatalf(t, "Did not get correct msg length: expected '%s' got '%s'\n", len, m[lenIndex])
	}
	if string(m[msgIndex]) != msg {
		stackFatalf(t, "Did not get correct msg: expected '%s' got '%s'\n", msg, m[msgIndex])
	}
}

// Closure for expectMsgs
func expectMsgsCommand(t tLogger, ef expectFun) func(int) [][][]byte {
	return func(expected int) [][][]byte {
		buf := ef(msgRe)
		matches := msgRe.FindAllSubmatch(buf, -1)
		if len(matches) != expected {
			stackFatalf(t, "Did not get correct # msgs: %d vs %d\n", len(matches), expected)
		}
		return matches
	}
}

// This will check that the matches include at least one of the sids. Useful for checking
// that we received messages on a certain queue group.
func checkForQueueSid(t tLogger, matches [][][]byte, sids []string) {
	seen := make(map[string]int, len(sids))
	for _, sid := range sids {
		seen[sid] = 0
	}
	for _, m := range matches {
		sid := string(m[sidIndex])
		if _, ok := seen[sid]; ok {
			seen[sid]++
		}
	}
	// Make sure we only see one and exactly one.
	total := 0
	for _, n := range seen {
		total += n
	}
	if total != 1 {
		stackFatalf(t, "Did not get a msg for queue sids group: expected 1 got %d\n", total)
	}
}

// This will check that the matches include all of the sids. Useful for checking
// that we received messages on all subscribers.
func checkForPubSids(t tLogger, matches [][][]byte, sids []string) {
	seen := make(map[string]int, len(sids))
	for _, sid := range sids {
		seen[sid] = 0
	}
	for _, m := range matches {
		sid := string(m[sidIndex])
		if _, ok := seen[sid]; ok {
			seen[sid]++
		}
	}
	// Make sure we only see one and exactly one for each sid.
	for sid, n := range seen {
		if n != 1 {
			stackFatalf(t, "Did not get a msg for sid[%s]: expected 1 got %d\n", sid, n)

		}
	}
}

// Helper function to generate next opts to make sure no port conflicts etc.
func nextServerOpts(opts *server.Options) *server.Options {
	nopts := *opts
	nopts.Port++
	nopts.Cluster.Port++
	nopts.HTTPPort++
	return &nopts
}
