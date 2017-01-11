// Copyright 2012-2016 Apcera Inc. All rights reserved.

package server

import (
	"time"
)

const (
	// VERSION is the current version for the server.
	VERSION = "0.9.6"

	// DEFAULT_PORT is the default port for client connections.
	DEFAULT_PORT = 4222

	// RANDOM_PORT is the value for port that, when supplied, will cause the
	// server to listen on a randomly-chosen available port. The resolved port
	// is available via the Addr() method.
	RANDOM_PORT = -1

	// DEFAULT_HOST defaults to all interfaces.
	DEFAULT_HOST = "0.0.0.0"

	// MAX_CONTROL_LINE_SIZE is the maximum allowed protocol control line size.
	// 1k should be plenty since payloads sans connect string are separate
	MAX_CONTROL_LINE_SIZE = 1024

	// MAX_PAYLOAD_SIZE is the maximum allowed payload size. Should be using
	// something different if > 1MB payloads are needed.
	MAX_PAYLOAD_SIZE = (1024 * 1024)

	// DEFAULT_MAX_CONNECTIONS is the default maximum connections allowed.
	DEFAULT_MAX_CONNECTIONS = (64 * 1024)

	// TLS_TIMEOUT is the TLS wait time.
	TLS_TIMEOUT = 500 * time.Millisecond

	// AUTH_TIMEOUT is the authorization wait time.
	AUTH_TIMEOUT = 2 * TLS_TIMEOUT

	// DEFAULT_PING_INTERVAL is how often pings are sent to clients and routes.
	DEFAULT_PING_INTERVAL = 2 * time.Minute

	// DEFAULT_PING_MAX_OUT is maximum allowed pings outstanding before disconnect.
	DEFAULT_PING_MAX_OUT = 2

	// CR_LF string
	CR_LF = "\r\n"

	// LEN_CR_LF hold onto the computed size.
	LEN_CR_LF = len(CR_LF)

	// DEFAULT_FLUSH_DEADLINE is the write/flush deadlines.
	DEFAULT_FLUSH_DEADLINE = 2 * time.Second

	// DEFAULT_HTTP_PORT is the default monitoring port.
	DEFAULT_HTTP_PORT = 8222

	// ACCEPT_MIN_SLEEP is the minimum acceptable sleep times on temporary errors.
	ACCEPT_MIN_SLEEP = 10 * time.Millisecond

	// ACCEPT_MAX_SLEEP is the maximum acceptable sleep times on temporary errors
	ACCEPT_MAX_SLEEP = 1 * time.Second

	// DEFAULT_ROUTE_CONNECT Route solicitation intervals.
	DEFAULT_ROUTE_CONNECT = 1 * time.Second

	// DEFAULT_ROUTE_RECONNECT Route reconnect intervals.
	DEFAULT_ROUTE_RECONNECT = 1 * time.Second

	// DEFAULT_ROUTE_DIAL Route dial timeout.
	DEFAULT_ROUTE_DIAL = 1 * time.Second

	// PROTO_SNIPPET_SIZE is the default size of proto to print on parse errors.
	PROTO_SNIPPET_SIZE = 32

	// MAX_MSG_ARGS Maximum possible number of arguments from MSG proto.
	MAX_MSG_ARGS = 4

	// MAX_PUB_ARGS Maximum possible number of arguments from PUB proto.
	MAX_PUB_ARGS = 3
)
