// Copyright 2016 Apcera Inc. All rights reserved.

package stores

import (
	"fmt"
	"sync"

	"github.com/nats-io/go-nats-streaming/pb"
	"github.com/nats-io/nats-streaming-server/spb"
)

// format string used to report that limit is reached when storing
// messages.
var droppingMsgsFmt = "WARNING: Reached limits for store %q (msgs=%v/%v bytes=%v/%v), " +
	"dropping old messages to make room for new ones."

// commonStore contains everything that is common to any type of store
type commonStore struct {
	sync.RWMutex
	closed bool
}

// genericStore is the generic store implementation with a map of channels.
type genericStore struct {
	commonStore
	limits   StoreLimits
	name     string
	channels map[string]*ChannelStore
	clients  map[string]*Client
}

// genericSubStore is the generic store implementation that manages subscriptions
// for a given channel.
type genericSubStore struct {
	commonStore
	limits    SubStoreLimits
	subject   string // Can't be wildcard
	subsCount int
	maxSubID  uint64
}

// genericMsgStore is the generic store implementation that manages messages
// for a given channel.
type genericMsgStore struct {
	commonStore
	limits     MsgStoreLimits
	subject    string // Can't be wildcard
	first      uint64
	last       uint64
	totalCount int
	totalBytes uint64
	hitLimit   bool // indicates if store had to drop messages due to limit
}

////////////////////////////////////////////////////////////////////////////
// genericStore methods
////////////////////////////////////////////////////////////////////////////

// init initializes the structure of a generic store
func (gs *genericStore) init(name string, limits *StoreLimits) {
	gs.name = name
	if limits == nil {
		limits = &DefaultStoreLimits
	}
	gs.setLimits(limits)
	// Do not use limits values to create the map.
	gs.channels = make(map[string]*ChannelStore)
	gs.clients = make(map[string]*Client)
}

// Init can be used to initialize the store with server's information.
func (gs *genericStore) Init(info *spb.ServerInfo) error {
	return nil
}

// Name returns the type name of this store
func (gs *genericStore) Name() string {
	return gs.name
}

// setLimits makes a copy of the given StoreLimits,
// validates the limits and if ok, applies the inheritance.
func (gs *genericStore) setLimits(limits *StoreLimits) error {
	// Make a copy
	gs.limits = *limits
	// of the map too
	if len(limits.PerChannel) > 0 {
		gs.limits.PerChannel = make(map[string]*ChannelLimits, len(limits.PerChannel))
		for key, val := range limits.PerChannel {
			// Make a copy of the values. We want ownership
			// of those structures
			gs.limits.PerChannel[key] = &(*val)
		}
	}
	// Build will validate and apply inheritance if no error.
	sl := &gs.limits
	return sl.Build()
}

// SetLimits sets limits for this store
func (gs *genericStore) SetLimits(limits *StoreLimits) error {
	gs.Lock()
	err := gs.setLimits(limits)
	gs.Unlock()
	return err
}

// CreateChannel creates a ChannelStore for the given channel, and returns
// `true` to indicate that the channel is new, false if it already exists.
func (gs *genericStore) CreateChannel(channel string, userData interface{}) (*ChannelStore, bool, error) {
	// no-op
	return nil, false, fmt.Errorf("Generic store, feature not implemented")
}

// LookupChannel returns a ChannelStore for the given channel.
func (gs *genericStore) LookupChannel(channel string) *ChannelStore {
	gs.RLock()
	cs := gs.channels[channel]
	gs.RUnlock()
	return cs
}

// HasChannel returns true if this store has any channel
func (gs *genericStore) HasChannel() bool {
	gs.RLock()
	l := len(gs.channels)
	gs.RUnlock()
	return l > 0
}

// State returns message store statistics for a given channel ('*' for all)
func (gs *genericStore) MsgsState(channel string) (numMessages int, byteSize uint64, err error) {
	numMessages = 0
	byteSize = 0
	err = nil

	if channel == AllChannels {
		gs.RLock()
		cs := gs.channels
		gs.RUnlock()

		for _, c := range cs {
			n, b, lerr := c.Msgs.State()
			if lerr != nil {
				err = lerr
				return
			}
			numMessages += n
			byteSize += b
		}
	} else {
		cs := gs.LookupChannel(channel)
		if cs != nil {
			numMessages, byteSize, err = cs.Msgs.State()
		}
	}
	return
}

// canAddChannel returns true if the current number of channels is below the limit.
// Store lock is assumed to be locked.
func (gs *genericStore) canAddChannel() error {
	if gs.limits.MaxChannels > 0 && len(gs.channels) >= gs.limits.MaxChannels {
		return ErrTooManyChannels
	}
	return nil
}

// AddClient stores information about the client identified by `clientID`.
func (gs *genericStore) AddClient(clientID, hbInbox string, userData interface{}) (*Client, bool, error) {
	c := &Client{spb.ClientInfo{ID: clientID, HbInbox: hbInbox}, userData}
	gs.Lock()
	oldClient := gs.clients[clientID]
	if oldClient != nil {
		gs.Unlock()
		return oldClient, false, nil
	}
	gs.clients[c.ID] = c
	gs.Unlock()
	return c, true, nil
}

// GetClient returns the stored Client, or nil if it does not exist.
func (gs *genericStore) GetClient(clientID string) *Client {
	gs.RLock()
	c := gs.clients[clientID]
	gs.RUnlock()
	return c
}

// GetClients returns all stored Client objects, as a map keyed by client IDs.
func (gs *genericStore) GetClients() map[string]*Client {
	gs.RLock()
	clients := make(map[string]*Client, len(gs.clients))
	for k, v := range gs.clients {
		clients[k] = v
	}
	gs.RUnlock()
	return clients
}

// GetClientsCount returns the number of registered clients
func (gs *genericStore) GetClientsCount() int {
	gs.RLock()
	count := len(gs.clients)
	gs.RUnlock()
	return count
}

// DeleteClient deletes the client identified by `clientID`.
func (gs *genericStore) DeleteClient(clientID string) *Client {
	gs.Lock()
	c := gs.clients[clientID]
	if c != nil {
		delete(gs.clients, clientID)
	}
	gs.Unlock()
	return c
}

// Close closes all stores
func (gs *genericStore) Close() error {
	gs.Lock()
	defer gs.Unlock()
	if gs.closed {
		return nil
	}
	gs.closed = true
	return gs.close()
}

// close closes all stores. Store lock is assumed held on entry
func (gs *genericStore) close() error {
	var err error
	var lerr error

	for _, cs := range gs.channels {
		lerr = cs.Subs.Close()
		if lerr != nil && err == nil {
			err = lerr
		}
		lerr = cs.Msgs.Close()
		if lerr != nil && err == nil {
			err = lerr
		}
	}
	return err
}

////////////////////////////////////////////////////////////////////////////
// genericMsgStore methods
////////////////////////////////////////////////////////////////////////////

// init initializes this generic message store
func (gms *genericMsgStore) init(subject string, limits *MsgStoreLimits) {
	gms.subject = subject
	gms.limits = *limits
}

// State returns some statistics related to this store
func (gms *genericMsgStore) State() (numMessages int, byteSize uint64, err error) {
	gms.RLock()
	c, b := gms.totalCount, gms.totalBytes
	gms.RUnlock()
	return c, b, nil
}

// FirstSequence returns sequence for first message stored.
func (gms *genericMsgStore) FirstSequence() uint64 {
	gms.RLock()
	first := gms.first
	gms.RUnlock()
	return first
}

// LastSequence returns sequence for last message stored.
func (gms *genericMsgStore) LastSequence() uint64 {
	gms.RLock()
	last := gms.last
	gms.RUnlock()
	return last
}

// FirstAndLastSequence returns sequences for the first and last messages stored.
func (gms *genericMsgStore) FirstAndLastSequence() (uint64, uint64) {
	gms.RLock()
	first, last := gms.first, gms.last
	gms.RUnlock()
	return first, last
}

// Lookup returns the stored message with given sequence number.
func (gms *genericMsgStore) Lookup(seq uint64) *pb.MsgProto {
	// no-op
	return nil
}

// FirstMsg returns the first message stored.
func (gms *genericMsgStore) FirstMsg() *pb.MsgProto {
	// no-op
	return nil
}

// LastMsg returns the last message stored.
func (gms *genericMsgStore) LastMsg() *pb.MsgProto {
	// no-op
	return nil
}

func (gms *genericMsgStore) Flush() error {
	// no-op
	return nil
}

// GetSequenceFromTimestamp returns the sequence of the first message whose
// timestamp is greater or equal to given timestamp.
func (gms *genericMsgStore) GetSequenceFromTimestamp(timestamp int64) uint64 {
	// no-op
	return 0
}

// Close closes this store.
func (gms *genericMsgStore) Close() error {
	return nil
}

////////////////////////////////////////////////////////////////////////////
// genericSubStore methods
////////////////////////////////////////////////////////////////////////////

// init initializes the structure of a generic sub store
func (gss *genericSubStore) init(channel string, limits *SubStoreLimits) {
	gss.subject = channel
	gss.limits = *limits
}

// CreateSub records a new subscription represented by SubState. On success,
// it records the subscription's ID in SubState.ID. This ID is to be used
// by the other SubStore methods.
func (gss *genericSubStore) CreateSub(sub *spb.SubState) error {
	gss.Lock()
	err := gss.createSub(sub)
	gss.Unlock()
	return err
}

// UpdateSub updates a given subscription represented by SubState.
func (gss *genericSubStore) UpdateSub(sub *spb.SubState) error {
	return nil
}

// createSub is the unlocked version of CreateSub that can be used by
// non-generic implementations.
func (gss *genericSubStore) createSub(sub *spb.SubState) error {
	if gss.limits.MaxSubscriptions > 0 && gss.subsCount >= gss.limits.MaxSubscriptions {
		return ErrTooManySubs
	}

	// Bump the max value before assigning it to the new subscription.
	gss.maxSubID++
	gss.subsCount++

	// This new subscription has the max value.
	sub.ID = gss.maxSubID

	return nil
}

// DeleteSub invalidates this subscription.
func (gss *genericSubStore) DeleteSub(subid uint64) {
	gss.Lock()
	gss.subsCount--
	gss.Unlock()
}

// AddSeqPending adds the given message seqno to the given subscription.
func (gss *genericSubStore) AddSeqPending(subid, seqno uint64) error {
	// no-op
	return nil
}

// AckSeqPending records that the given message seqno has been acknowledged
// by the given subscription.
func (gss *genericSubStore) AckSeqPending(subid, seqno uint64) error {
	// no-op
	return nil
}

// Flush is for stores that may buffer operations and need them to be persisted.
func (gss *genericSubStore) Flush() error {
	// no-op
	return nil
}

// Close closes this store
func (gss *genericSubStore) Close() error {
	// no-op
	return nil
}
