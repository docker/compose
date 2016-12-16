// Copyright 2012-2014 Apcera Inc. All rights reserved.

package server

// Auth is an interface for implementing authentication
type Auth interface {
	// Check if a client is authorized to connect
	Check(c ClientAuth) bool
}

// ClientAuth is an interface for client authentication
type ClientAuth interface {
	// Get options associated with a client
	GetOpts() *clientOpts
	// Optionally map a user after auth.
	RegisterUser(*User)
}
