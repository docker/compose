// Copyright 2015-2016 Apcera Inc. All rights reserved.

package pse

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func ProcUsage(pcpu *float64, rss, vss *int64) error {
	pidStr := fmt.Sprintf("%d", os.Getpid())
	out, err := exec.Command("ps", "o", "pcpu=,rss=,vsz=", "-p", pidStr).Output()
	if err != nil {
		*rss, *vss = -1, -1
		return errors.New(fmt.Sprintf("ps call failed:%v", err))
	}
	fmt.Sscanf(string(out), "%f %d %d", pcpu, rss, vss)
	*rss *= 1024 // 1k blocks, want bytes.
	*vss *= 1024 // 1k blocks, want bytes.
	return nil
}
