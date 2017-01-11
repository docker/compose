// Copyright 2013-2016 Apcera Inc. All rights reserved.

package server

// SortOpt is a helper type to sort by ConnInfo values
type SortOpt string

const (
	byCid      SortOpt = "cid"
	bySubs             = "subs"
	byPending          = "pending"
	byOutMsgs          = "msgs_to"
	byInMsgs           = "msgs_from"
	byOutBytes         = "bytes_to"
	byInBytes          = "bytes_from"
	byLast             = "last"
	byIdle             = "idle"
	byUptime           = "uptime"
)

// IsValid determines if a sort option is valid
func (s SortOpt) IsValid() bool {
	switch s {
	case "", byCid, bySubs, byPending, byOutMsgs, byInMsgs, byOutBytes, byInBytes, byLast, byIdle, byUptime:
		return true
	default:
		return false
	}
}

// Pair type is internally used.
type Pair struct {
	Key *client
	Val int64
}

// Pairs type is internally used.
type Pairs []Pair

func (d Pairs) Len() int {
	return len(d)
}

func (d Pairs) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}

func (d Pairs) Less(i, j int) bool {
	return d[i].Val < d[j].Val
}
