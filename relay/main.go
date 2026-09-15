/*
   Copyright 2020 Docker Compose CLI authors

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

// compose-relay is the network gateway Docker Compose deploys in place of a
// provider-managed service. It joins the project networks under the service's
// name and forwards each declared container port to the endpoint the provider
// published (typically a port on the host gateway), so consumers keep using
// the compose-native address (http://<service>:<port>) for a resource that
// lives outside the compose network.
//
// Routes come from the RELAY_ROUTES environment variable:
//
//	RELAY_ROUTES=80=host.docker.internal:49152,9000=host.docker.internal:30001
//
// One listener per route, TCP only.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	routes, err := parseRoutes(os.Getenv("RELAY_ROUTES"))
	if err != nil {
		log.Fatalf("RELAY_ROUTES: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var wg sync.WaitGroup
	for port, upstream := range routes {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			log.Fatalf("listen :%d: %v", port, err)
		}
		log.Printf("relaying :%d -> %s", port, upstream)
		wg.Add(1)
		go func() {
			defer wg.Done()
			serve(ctx, listener, upstream, &wg)
		}()
		go func() {
			<-ctx.Done()
			_ = listener.Close()
		}()
	}
	wg.Wait()
}

// parseRoutes decodes "port=host:port[,port=host:port...]".
//
// A provider announces endpoints as seen from ITS host ("localhost:49152"):
// where a resource actually lives is not its concern to translate. The relay
// is the component that knows it runs inside a container, so a loopback (or
// unspecified) upstream host is rewritten here to host.docker.internal — the
// container-visible name for the host, resolved through the host-gateway
// extra_host compose injects. Routable addresses pass through untouched.
func parseRoutes(spec string) (map[int]string, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("no routes configured")
	}
	routes := map[int]string{}
	for _, entry := range strings.Split(spec, ",") {
		portPart, upstream, found := strings.Cut(entry, "=")
		if !found {
			return nil, fmt.Errorf("invalid route %q (want port=host:port)", entry)
		}
		port, err := strconv.Atoi(portPart)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid port in route %q", entry)
		}
		host, hostPort, err := net.SplitHostPort(upstream)
		if err != nil {
			return nil, fmt.Errorf("invalid upstream in route %q: %v", entry, err)
		}
		if hostIsContainerLocal(host) {
			upstream = net.JoinHostPort("host.docker.internal", hostPort)
		}
		routes[port] = upstream
	}
	return routes, nil
}

// hostIsContainerLocal reports whether a host announced by the provider
// designates the provider's own host machine (loopback or unspecified) —
// unreachable under that name from inside a container.
func hostIsContainerLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// serve accepts connections until the listener closes. In-flight forwards
// join the WaitGroup, so on SIGTERM the process stops accepting (listeners
// close) but drains established connections until they finish — or until the
// engine's stop timeout escalates to SIGKILL.
func serve(ctx context.Context, listener net.Listener, upstream string, wg *sync.WaitGroup) {
	backoff := 5 * time.Millisecond
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Back off before retrying: some accept errors (e.g. EMFILE)
			// persist until a descriptor is freed, and a tight loop would
			// spin the CPU and flood the log.
			log.Printf("accept on %s: %v", listener.Addr(), err)
			time.Sleep(backoff)
			if backoff < time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 5 * time.Millisecond
		wg.Add(1)
		go func() {
			defer wg.Done()
			forward(conn, upstream)
		}()
	}
}

// halfCloseIdleTimeout bounds how long the surviving direction may sit IDLE
// once the other one has finished. Half-close semantics stay intact — a peer
// that keeps sending data after the other side's FIN is relayed for as long
// as it takes — but a peer that never closes after our FIN can no longer
// pin the goroutine pair and both connections forever (and with them the
// drain in main). A variable so tests exercise the expiry quickly.
var halfCloseIdleTimeout = 60 * time.Second

// idleConn re-arms a read deadline before each Read once armed: active
// transfers never expire, idle ones do.
type idleConn struct {
	net.Conn
	armed atomic.Bool
}

func (c *idleConn) Read(p []byte) (int, error) {
	if c.armed.Load() {
		_ = c.Conn.SetReadDeadline(time.Now().Add(halfCloseIdleTimeout))
	}
	return c.Conn.Read(p)
}

// forward deliberately takes no context: a connection accepted at the
// shutdown boundary (context cancelled, listener not yet closed) must still
// be served — that is the drain contract — and a cancelled context would make
// DialContext fail instantly. The dialer timeout bounds the dial instead.
func forward(downstream net.Conn, upstream string) {
	defer downstream.Close()
	dialer := net.Dialer{Timeout: 10 * time.Second}
	up, err := dialer.Dial("tcp", upstream)
	if err != nil {
		log.Printf("dial %s: %v", upstream, err)
		return
	}
	defer up.Close()

	down := &idleConn{Conn: downstream}
	upc := &idleConn{Conn: up}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, down); closeWrite(up); done <- struct{}{} }()
	go func() { _, _ = io.Copy(downstream, upc); closeWrite(downstream); done <- struct{}{} }()
	<-done
	// One direction is done: from here on the survivor may stream for as
	// long as data flows, but no longer sit idle forever. Arm the per-read
	// deadline for its future Reads, and set one immediately for a survivor
	// already blocked in Read.
	down.armed.Store(true)
	upc.armed.Store(true)
	deadline := time.Now().Add(halfCloseIdleTimeout)
	_ = downstream.SetReadDeadline(deadline)
	_ = up.SetReadDeadline(deadline)
	<-done
}

// closeWrite propagates half-closes so protocols relying on EOF work through
// the relay.
func closeWrite(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
}
