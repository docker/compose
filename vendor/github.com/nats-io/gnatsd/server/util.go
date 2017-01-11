// Copyright 2012-2016 Apcera Inc. All rights reserved.

package server

import (
	"time"

	"github.com/nats-io/nuid"
)

// Use nuid.
func genID() string {
	return nuid.Next()
}

// Ascii numbers 0-9
const (
	asciiZero = 48
	asciiNine = 57
)

// parseSize expects decimal positive numbers. We
// return -1 to signal error
func parseSize(d []byte) (n int) {
	if len(d) == 0 {
		return -1
	}
	for _, dec := range d {
		if dec < asciiZero || dec > asciiNine {
			return -1
		}
		n = n*10 + (int(dec) - asciiZero)
	}
	return n
}

// parseInt64 expects decimal positive numbers. We
// return -1 to signal error
func parseInt64(d []byte) (n int64) {
	if len(d) == 0 {
		return -1
	}
	for _, dec := range d {
		if dec < asciiZero || dec > asciiNine {
			return -1
		}
		n = n*10 + (int64(dec) - asciiZero)
	}
	return n
}

// Helper to move from float seconds to time.Duration
func secondsToDuration(seconds float64) time.Duration {
	ttl := seconds * float64(time.Second)
	return time.Duration(ttl)
}
