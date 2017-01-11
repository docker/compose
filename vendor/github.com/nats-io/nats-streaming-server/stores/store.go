// Copyright 2016 Apcera Inc. All rights reserved.

package stores

import (
	"errors"
	"time"

	"github.com/nats-io/gnatsd/server"
	"github.com/nats-io/go-nats-streaming/pb"
	"github.com/nats-io/nats-streaming-server/spb"
)

const (
	// TypeMemory is the store type name for memory based stores
	TypeMemory = "MEMORY"
	// TypeFile is the store type name for file based stores
	TypeFile = "FILE"
)

const (
	// AllChannels allows to get state for all channels.
	AllChannels = "*"
)

// Errors.
var (
	ErrTooManyChannels = errors.New("too many channels")
	ErrTooManySubs     = errors.New("too many subscriptions per channel")
)

// Noticef logs a notice statement
func Noticef(format string, v ...interface{}) {
	server.Noticef(format, v...)
}

// StoreLimits define limits for a store.
type StoreLimits struct {
	// How many channels are allowed.
	MaxChannels int
	// Global limits. Any 0 value means that the limit is ignored (unlimited).
	ChannelLimits
	// Per-channel limits. If a limit for a channel in this map is 0,
	// the corresponding global limit (specified above) is used.
	PerChannel map[string]*ChannelLimits
}

// ChannelLimits defines limits for a given channel
type ChannelLimits struct {
	// Limits for message stores
	MsgStoreLimits
	// Limits for subscriptions stores
	SubStoreLimits
}

// MsgStoreLimits defines limits for a MsgStore.
// For global limits, a value of 0 means "unlimited".
// For per-channel limits, it means that the corresponding global
// limit is used.
type MsgStoreLimits struct {
	// How many messages are allowed.
	MaxMsgs int
	// How many bytes are allowed.
	MaxBytes int64
	// How long messages are kept in the log (unit is seconds)
	MaxAge time.Duration
}

// SubStoreLimits defines limits for a SubStore
type SubStoreLimits struct {
	// How many subscriptions are allowed.
	MaxSubscriptions int
}

// DefaultStoreLimits are the limits that a Store must
// use when none are specified to the Store constructor.
// Store limits can be changed with the Store.SetLimits() method.
var DefaultStoreLimits = StoreLimits{
	100,
	ChannelLimits{
		MsgStoreLimits{
			MaxMsgs:  1000000,
			MaxBytes: 1000000 * 1024,
		},
		SubStoreLimits{
			MaxSubscriptions: 1000,
		},
	},
	nil,
}

// RecoveredState allows the server to reconstruct its state after a restart.
type RecoveredState struct {
	Info    *spb.ServerInfo
	Clients []*Client
	Subs    RecoveredSubscriptions
}

// Client represents a client with ID, Heartbeat Inbox and user data sets
// when adding it to the store.
type Client struct {
	spb.ClientInfo
	UserData interface{}
}

// RecoveredSubscriptions is a map of recovered subscriptions, keyed by channel name.
type RecoveredSubscriptions map[string][]*RecoveredSubState

// PendingAcks is a set of message sequences waiting to be acknowledged.
type PendingAcks map[uint64]struct{}

// RecoveredSubState represents a recovered Subscription with a map
// of pending messages.
type RecoveredSubState struct {
	Sub     *spb.SubState
	Pending PendingAcks
}

// ChannelStore contains a reference to both Subscription and Message stores.
type ChannelStore struct {
	// UserData is set when the channel is created.
	UserData interface{}
	// Subs is the Subscriptions Store.
	Subs SubStore
	// Msgs is the Messages Store.
	Msgs MsgStore
}

// Store is the storage interface for NATS Streaming servers.
//
// If an implementation has a Store constructor with StoreLimits, it should be
// noted that the limits don't apply to any state being recovered, for Store
// implementations supporting recovery.
//
type Store interface {
	// Init can be used to initialize the store with server's information.
	Init(info *spb.ServerInfo) error

	// Name returns the name type of this store (e.g: MEMORY, FILESTORE, etc...).
	Name() string

	// SetLimits sets limits for this store. The action is not expected
	// to be retroactive.
	// The store implementation should make a deep copy as to not change
	// the content of the structure passed by the caller.
	// This call may return an error due to limits validation errors.
	SetLimits(limits *StoreLimits) error

	// CreateChannel creates a ChannelStore for the given channel, and returns
	// `true` to indicate that the channel is new, false if it already exists.
	// Limits defined for this channel in StoreLimits.PeChannel map, if present,
	// will apply. Otherwise, the global limits in StoreLimits will apply.
	CreateChannel(channel string, userData interface{}) (*ChannelStore, bool, error)

	// LookupChannel returns a ChannelStore for the given channel, nil if channel
	// does not exist.
	LookupChannel(channel string) *ChannelStore

	// HasChannel returns true if this store has any channel.
	HasChannel() bool

	// MsgsState returns message store statistics for a given channel, or all
	// if 'channel' is AllChannels.
	MsgsState(channel string) (numMessages int, byteSize uint64, err error)

	// AddClient stores information about the client identified by `clientID`.
	// If a Client is already registered, this call returns the currently
	// registered Client object, and the boolean set to false to indicate
	// that the client is not new.
	AddClient(clientID, hbInbox string, userData interface{}) (*Client, bool, error)

	// GetClient returns the stored Client, or nil if it does not exist.
	GetClient(clientID string) *Client

	// GetClients returns a map of all stored Client objects, keyed by client IDs.
	// The returned map is a copy of the state maintained by the store so that
	// it is safe for the caller to walk through the map while clients may be
	// added/deleted from the store.
	GetClients() map[string]*Client

	// GetClientsCount returns the number of registered clients.
	GetClientsCount() int

	// DeleteClient removes the client identified by `clientID` from the store
	// and returns it to the caller.
	DeleteClient(clientID string) *Client

	// Close closes all stores.
	Close() error
}

// SubStore is the interface for storage of Subscriptions on a given channel.
//
// Implementations of this interface should not attempt to validate that
// a subscription is valid (that is, has not been deleted) when processing
// updates.
type SubStore interface {
	// CreateSub records a new subscription represented by SubState. On success,
	// it records the subscription's ID in SubState.ID. This ID is to be used
	// by the other SubStore methods.
	CreateSub(*spb.SubState) error

	// UpdateSub updates a given subscription represented by SubState.
	UpdateSub(*spb.SubState) error

	// DeleteSub invalidates the subscription 'subid'.
	DeleteSub(subid uint64)

	// AddSeqPending adds the given message 'seqno' to the subscription 'subid'.
	AddSeqPending(subid, seqno uint64) error

	// AckSeqPending records that the given message 'seqno' has been acknowledged
	// by the subscription 'subid'.
	AckSeqPending(subid, seqno uint64) error

	// Flush is for stores that may buffer operations and need them to be persisted.
	Flush() error

	// Close closes the subscriptions store.
	Close() error
}

// MsgStore is the interface for storage of Messages on a given channel.
type MsgStore interface {
	// State returns some statistics related to this store.
	State() (numMessages int, byteSize uint64, err error)

	// Store stores a message and returns the message sequence.
	Store(data []byte) (uint64, error)

	// Lookup returns the stored message with given sequence number.
	Lookup(seq uint64) *pb.MsgProto

	// FirstSequence returns sequence for first message stored, 0 if no
	// message is stored.
	FirstSequence() uint64

	// LastSequence returns sequence for last message stored, 0 if no
	// message is stored.
	LastSequence() uint64

	// FirstAndLastSequence returns sequences for the first and last messages stored,
	// 0 if no message is stored.
	FirstAndLastSequence() (uint64, uint64)

	// GetSequenceFromTimestamp returns the sequence of the first message whose
	// timestamp is greater or equal to given timestamp.
	GetSequenceFromTimestamp(timestamp int64) uint64

	// FirstMsg returns the first message stored.
	FirstMsg() *pb.MsgProto

	// LastMsg returns the last message stored.
	LastMsg() *pb.MsgProto

	// Flush is for stores that may buffer operations and need them to be persisted.
	Flush() error

	// Close closes the store.
	Close() error
}
