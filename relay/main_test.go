/*
   Copyright 2026 Docker Compose CLI authors

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

package main

import (
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

// A provider announces endpoints as seen from its host; the relay, which
// knows it runs inside a container, rewrites host-local upstreams to
// host.docker.internal and leaves routable addresses untouched.
func TestParseRoutesTranslatesHostLocalUpstreams(t *testing.T) {
	routes, err := parseRoutes("80=localhost:49152,81=127.0.0.1:5734,82=[::1]:5735,83=0.0.0.0:5736,443=192.168.1.10:8443")
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{
		80:  "host.docker.internal:49152",
		81:  "host.docker.internal:5734",
		82:  "host.docker.internal:5735",
		83:  "host.docker.internal:5736",
		443: "192.168.1.10:8443",
	}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("got %v, want %v", routes, want)
	}
}

func TestParseRoutesRejectsMalformedEntries(t *testing.T) {
	for _, spec := range []string{"", "80", "80=nohostport", "0=localhost:1", "x=localhost:1"} {
		if _, err := parseRoutes(spec); err == nil {
			t.Errorf("parseRoutes(%q): expected error", spec)
		}
	}
}

// tcpPair returns a connected (client, server) TCP pair.
func tcpPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	type accepted struct {
		conn net.Conn
		err  error
	}
	ch := make(chan accepted, 1)
	go func() {
		conn, err := ln.Accept()
		ch <- accepted{conn, err}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server := <-ch
	if server.err != nil {
		t.Fatal(server.err)
	}
	return client, server.conn
}

// silentUpstream accepts one connection, drains it, and sits silent with its
// write side open until closed.
func silentUpstream(t *testing.T) net.Listener {
	t.Helper()
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upstream.Close() })
	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(io.Discard, conn) // read the FIN, never answer, never close
		time.Sleep(5 * time.Second)
	}()
	return upstream
}

// A peer that never closes after receiving our FIN must not pin the forward
// forever: once one direction is done, the surviving one may stream for as
// long as data flows but is reaped when idle past the grace.
func TestForwardReapsIdleHalfOpenPair(t *testing.T) {
	restore := halfCloseIdleTimeout
	halfCloseIdleTimeout = 200 * time.Millisecond
	t.Cleanup(func() { halfCloseIdleTimeout = restore })

	upstream := silentUpstream(t)
	client, server := tcpPair(t)
	t.Cleanup(func() { _ = client.Close() })

	finished := make(chan struct{})
	go func() {
		forward(server, upstream.Addr().String())
		close(finished)
	}()

	if _, err := client.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	_ = client.Close() // downstream fully gone: down->up finishes, up->down survives

	select {
	case <-finished:
		// reaped by the idle grace instead of hanging on the silent upstream
	case <-time.After(5 * time.Second):
		t.Fatal("forward still blocked on a half-open pair after the idle grace")
	}
}

// Half-close semantics survive the reaper: a response still flowing after the
// client half-closed its request side is relayed past the grace, not cut.
func TestForwardKeepsStreamingAfterHalfClose(t *testing.T) {
	restore := halfCloseIdleTimeout
	halfCloseIdleTimeout = 300 * time.Millisecond
	t.Cleanup(func() { halfCloseIdleTimeout = restore })

	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upstream.Close() })
	const chunks = 6
	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// stream chunks for well past the idle grace, each within it
		for range chunks {
			time.Sleep(150 * time.Millisecond)
			if _, err := conn.Write([]byte("chunk")); err != nil {
				return
			}
		}
	}()

	client, server := tcpPair(t)
	t.Cleanup(func() { _ = client.Close() })
	finished := make(chan struct{})
	go func() {
		forward(server, upstream.Addr().String())
		close(finished)
	}()

	closeWrite(client) // request side done; the response keeps streaming

	got := 0
	buf := make([]byte, 64)
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		n, err := client.Read(buf)
		got += n
		if err != nil {
			break
		}
	}
	if got < chunks*len("chunk") {
		t.Fatalf("streaming was cut by the idle reaper: got %d bytes, want %d", got, chunks*len("chunk"))
	}
	// let forward return before the Cleanup restores the shared grace var
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("forward did not return after both directions ended")
	}
}
