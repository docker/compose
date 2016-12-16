// Copyright 2015 Apcera Inc. All rights reserved.

package pse

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	procStatFile string
	ticks        int64
	lastTotal    int64
	lastSeconds  int64
	ipcpu        int64
)

const (
	utimePos = 13
	stimePos = 14
	startPos = 21
	vssPos   = 22
	rssPos   = 23
)

func init() {
	// Avoiding to generate docker image without CGO
	ticks = 100 // int64(C.sysconf(C._SC_CLK_TCK))
	procStatFile = fmt.Sprintf("/proc/%d/stat", os.Getpid())
	periodic()
}

// Sampling function to keep pcpu relevant.
func periodic() {
	contents, err := ioutil.ReadFile(procStatFile)
	if err != nil {
		return
	}
	fields := bytes.Fields(contents)

	// PCPU
	pstart := parseInt64(fields[startPos])
	utime := parseInt64(fields[utimePos])
	stime := parseInt64(fields[stimePos])
	total := utime + stime

	var sysinfo syscall.Sysinfo_t
	if err := syscall.Sysinfo(&sysinfo); err != nil {
		return
	}

	seconds := int64(sysinfo.Uptime) - (pstart / ticks)

	// Save off temps
	lt := lastTotal
	ls := lastSeconds

	// Update last sample
	lastTotal = total
	lastSeconds = seconds

	// Adjust to current time window
	total -= lt
	seconds -= ls

	if seconds > 0 {
		atomic.StoreInt64(&ipcpu, (total*1000/ticks)/seconds)
	}

	time.AfterFunc(1*time.Second, periodic)
}

func ProcUsage(pcpu *float64, rss, vss *int64) error {
	contents, err := ioutil.ReadFile(procStatFile)
	if err != nil {
		return err
	}
	fields := bytes.Fields(contents)

	// Memory
	*rss = (parseInt64(fields[rssPos])) << 12
	*vss = parseInt64(fields[vssPos])

	// PCPU
	// We track this with periodic sampling, so just load and go.
	*pcpu = float64(atomic.LoadInt64(&ipcpu)) / 10.0

	return nil
}

// Ascii numbers 0-9
const (
	asciiZero = 48
	asciiNine = 57
)

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
