// Copyright 2016 Apcera Inc. All rights reserved.

package server

import (
	"github.com/nats-io/nats-streaming-server/stores"
	"sync"
	"time"
)

// This is a proxy to the store interface.
type clientStore struct {
	store stores.Store
}

// client has information needed by the server. A client is also
// stored in a stores.Client object (which contains ID and HbInbox).
type client struct {
	sync.RWMutex
	unregistered bool
	hbt          *time.Timer
	fhb          int
	subs         []*subState
}

// Register a client if new, otherwise returns the client already registered
// and `false` to indicate that the client is not new.
func (cs *clientStore) Register(ID, hbInbox string) (*stores.Client, bool, error) {
	// Will be gc'ed if we fail to register, that's ok.
	c := &client{subs: make([]*subState, 0, 4)}
	sc, isNew, err := cs.store.AddClient(ID, hbInbox, c)
	if err != nil {
		return nil, false, err
	}
	return sc, isNew, nil
}

// Unregister a client.
func (cs *clientStore) Unregister(ID string) *stores.Client {
	sc := cs.store.DeleteClient(ID)
	if sc != nil {
		c := sc.UserData.(*client)
		c.Lock()
		c.unregistered = true
		c.Unlock()
	}
	return sc
}

// IsValid returns true if the client is registered, false otherwise.
func (cs *clientStore) IsValid(ID string) bool {
	return cs.store.GetClient(ID) != nil
}

// Lookup a client
func (cs *clientStore) Lookup(ID string) *client {
	sc := cs.store.GetClient(ID)
	if sc != nil {
		return sc.UserData.(*client)
	}
	return nil
}

// GetSubs returns the list of subscriptions for the client identified by ID,
// or nil if such client is not found.
func (cs *clientStore) GetSubs(ID string) []*subState {
	c := cs.Lookup(ID)
	if c == nil {
		return nil
	}
	c.RLock()
	subs := make([]*subState, len(c.subs))
	copy(subs, c.subs)
	c.RUnlock()
	return subs
}

// AddSub adds the subscription to the client identified by clientID
// and returns true only if the client has not been unregistered,
// otherwise returns false.
func (cs *clientStore) AddSub(ID string, sub *subState) bool {
	sc := cs.store.GetClient(ID)
	if sc == nil {
		return false
	}
	c := sc.UserData.(*client)
	c.Lock()
	if c.unregistered {
		c.Unlock()
		return false
	}
	c.subs = append(c.subs, sub)
	c.Unlock()
	return true
}

// RemoveSub removes the subscription from the client identified by clientID
// and returns true only if the client has not been unregistered and that
// the subscription was found, otherwise returns false.
func (cs *clientStore) RemoveSub(ID string, sub *subState) bool {
	sc := cs.store.GetClient(ID)
	if sc == nil {
		return false
	}
	c := sc.UserData.(*client)
	c.Lock()
	if c.unregistered {
		c.Unlock()
		return false
	}
	removed := false
	c.subs, removed = sub.deleteFromList(c.subs)
	c.Unlock()
	return removed
}
