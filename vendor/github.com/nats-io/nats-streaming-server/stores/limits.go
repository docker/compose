// Copyright 2016 Apcera Inc. All rights reserved.

package stores

import (
	"fmt"
)

// AddPerChannel stores limits for the given channel `name` in the StoreLimits.
// Inheritance (that is, specifying 0 for a limit means that the global limit
// should be used) is not applied in this call. This is done in StoreLimits.Build
// along with some validation.
func (sl *StoreLimits) AddPerChannel(name string, cl *ChannelLimits) {
	if sl.PerChannel == nil {
		sl.PerChannel = make(map[string]*ChannelLimits)
	}
	sl.PerChannel[name] = cl
}

// Build sets the global limits into per-channel limits that are set
// to zero. This call also validates the limits. An error is returned if:
// * any limit is set to a negative value.
// * the number of per-channel is higher than StoreLimits.MaxChannels.
// * any per-channel limit is higher than the corresponding global limit.
func (sl *StoreLimits) Build() error {
	// Check that there is no negative value
	if sl.MaxChannels < 0 {
		return fmt.Errorf("Max channels limit cannot be negative")
	}
	if err := sl.checkChannelLimits(&sl.ChannelLimits, ""); err != nil {
		return err
	}
	// If there is no per-channel, we are done.
	if len(sl.PerChannel) == 0 {
		return nil
	}
	if len(sl.PerChannel) > sl.MaxChannels {
		return fmt.Errorf("Too many channels defined (%v). The max channels limit is set to %v",
			len(sl.PerChannel), sl.MaxChannels)
	}
	for cn, cl := range sl.PerChannel {
		if err := sl.checkChannelLimits(cl, cn); err != nil {
			return err
		}
	}
	// If we are here, it means that there was no error,
	// so we now apply inheritance.
	for _, cl := range sl.PerChannel {
		if cl.MaxSubscriptions == 0 {
			cl.MaxSubscriptions = sl.MaxSubscriptions
		}
		if cl.MaxMsgs == 0 {
			cl.MaxMsgs = sl.MaxMsgs
		}
		if cl.MaxBytes == 0 {
			cl.MaxBytes = sl.MaxBytes
		}
		if cl.MaxAge == 0 {
			cl.MaxAge = sl.MaxAge
		}
	}
	return nil
}

func (sl *StoreLimits) checkChannelLimits(cl *ChannelLimits, channelName string) error {
	// Check that there is no per-channel unlimited limit if corresponding
	// limit is not.
	if err := verifyLimit("subscriptions", channelName,
		int64(cl.MaxSubscriptions), int64(sl.MaxSubscriptions)); err != nil {
		return err
	}
	if err := verifyLimit("messages", channelName,
		int64(cl.MaxMsgs), int64(sl.MaxMsgs)); err != nil {
		return err
	}
	if err := verifyLimit("bytes", channelName,
		cl.MaxBytes, sl.MaxBytes); err != nil {
		return err
	}
	if err := verifyLimit("age", channelName,
		int64(cl.MaxAge), int64(sl.MaxAge)); err != nil {
		return err
	}
	return nil
}

func verifyLimit(errText, channelName string, limit, globalLimit int64) error {
	// No limit can be negative. If channelName is "" we are
	// verifying the global limit (in this case limit == globalLimit).
	// Otherwise, we verify a given per-channel limit. Make
	// sure that the value is not greater than the corresponding
	// global limit.
	if channelName == "" {
		if limit < 0 {
			return fmt.Errorf("Max %s for global limit cannot be negative", errText)
		}
		return nil
	}
	// Per-channel limit specific here.
	if limit < 0 {
		return fmt.Errorf("Max %s for channel %q cannot be negative. "+
			"Set it to 0 to be equal to the global limit of %v", errText, channelName, globalLimit)
	}
	if limit > globalLimit {
		return fmt.Errorf("Max %s for channel %q cannot be higher than global limit "+
			"of %v", errText, channelName, globalLimit)
	}
	return nil
}
