// Copyright 2015-2016 Apcera Inc. All rights reserved.
// +build rumprun

package pse

// This is a placeholder for now.
func ProcUsage(pcpu *float64, rss, vss *int64) error {
	*pcpu = 0.0
	*rss = 0
	*vss = 0

	return nil
}
