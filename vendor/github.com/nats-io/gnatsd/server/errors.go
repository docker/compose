// Copyright 2012-2016 Apcera Inc. All rights reserved.

package server

import "errors"

var (
	// ErrConnectionClosed represents an error condition on a closed connection.
	ErrConnectionClosed = errors.New("Connection Closed")

	// ErrAuthorization represents an error condition on failed authorization.
	ErrAuthorization = errors.New("Authorization Error")

	// ErrAuthTimeout represents an error condition on failed authorization due to timeout.
	ErrAuthTimeout = errors.New("Authorization Timeout")

	// ErrMaxPayload represents an error condition when the payload is too big.
	ErrMaxPayload = errors.New("Maximum Payload Exceeded")

	// ErrMaxControlLine represents an error condition when the control line is too big.
	ErrMaxControlLine = errors.New("Maximum Control Line Exceeded")

	// ErrReservedPublishSubject represents an error condition when sending to a reserved subject, e.g. _SYS.>
	ErrReservedPublishSubject = errors.New("Reserved Internal Subject")

	// ErrBadClientProtocol signals a client requested an invalud client protocol.
	ErrBadClientProtocol = errors.New("Invalid Client Protocol")

	// ErrTooManyConnections signals a client that the maximum number of connections supported by the
	// server has been reached.
	ErrTooManyConnections = errors.New("Maximum Connections Exceeded")
)
