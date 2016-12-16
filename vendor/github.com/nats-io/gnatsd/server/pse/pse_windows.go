// Copyright 2015-2016 Apcera Inc. All rights reserved.
// +build windows

package pse

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	pdh                            = syscall.NewLazyDLL("pdh.dll")
	winPdhOpenQuery                = pdh.NewProc("PdhOpenQuery")
	winPdhAddCounter               = pdh.NewProc("PdhAddCounterW")
	winPdhCollectQueryData         = pdh.NewProc("PdhCollectQueryData")
	winPdhGetFormattedCounterValue = pdh.NewProc("PdhGetFormattedCounterValue")
	winPdhGetFormattedCounterArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
)

// global performance counter query handle and counters
var (
	pcHandle                                       PDH_HQUERY
	pidCounter, cpuCounter, rssCounter, vssCounter PDH_HCOUNTER
	prevCPU                                        float64
	prevRss                                        int64
	prevVss                                        int64
	lastSampleTime                                 time.Time
	processPid                                     int
	pcQueryLock                                    sync.Mutex
	initialSample                                  = true
)

// maxQuerySize is the number of values to return from a query.
// It represents the maximum # of servers that can be queried
// simultaneously running on a machine.
const maxQuerySize = 512

// Keep static memory around to reuse; this works best for passing
// into the pdh API.
var counterResults [maxQuerySize]PDH_FMT_COUNTERVALUE_ITEM_DOUBLE

// PDH Types
type (
	PDH_HQUERY   syscall.Handle
	PDH_HCOUNTER syscall.Handle
)

// PDH constants used here
const (
	PDH_FMT_DOUBLE   = 0x00000200
	PDH_INVALID_DATA = 0xC0000BC6
	PDH_MORE_DATA    = 0x800007D2
)

// PDH_FMT_COUNTERVALUE_DOUBLE - double value
type PDH_FMT_COUNTERVALUE_DOUBLE struct {
	CStatus     uint32
	DoubleValue float64
}

// PDH_FMT_COUNTERVALUE_ITEM_DOUBLE is an array
// element of a double value
type PDH_FMT_COUNTERVALUE_ITEM_DOUBLE struct {
	SzName   *uint16 // pointer to a string
	FmtValue PDH_FMT_COUNTERVALUE_DOUBLE
}

func pdhAddCounter(hQuery PDH_HQUERY, szFullCounterPath string, dwUserData uintptr, phCounter *PDH_HCOUNTER) error {
	ptxt, _ := syscall.UTF16PtrFromString(szFullCounterPath)
	r0, _, _ := winPdhAddCounter.Call(
		uintptr(hQuery),
		uintptr(unsafe.Pointer(ptxt)),
		dwUserData,
		uintptr(unsafe.Pointer(phCounter)))

	if r0 != 0 {
		return fmt.Errorf("pdhAddCounter failed. %d", r0)
	}
	return nil
}

func pdhOpenQuery(datasrc *uint16, userdata uint32, query *PDH_HQUERY) error {
	r0, _, _ := syscall.Syscall(winPdhOpenQuery.Addr(), 3, 0, uintptr(userdata), uintptr(unsafe.Pointer(query)))
	if r0 != 0 {
		return fmt.Errorf("pdhOpenQuery failed - %d", r0)
	}
	return nil
}

func pdhCollectQueryData(hQuery PDH_HQUERY) error {
	r0, _, _ := winPdhCollectQueryData.Call(uintptr(hQuery))
	if r0 != 0 {
		return fmt.Errorf("pdhCollectQueryData failed - %d", r0)
	}
	return nil
}

// pdhGetFormattedCounterArrayDouble returns the value of return code
// rather than error, to easily check return codes
func pdhGetFormattedCounterArrayDouble(hCounter PDH_HCOUNTER, lpdwBufferSize *uint32, lpdwBufferCount *uint32, itemBuffer *PDH_FMT_COUNTERVALUE_ITEM_DOUBLE) uint32 {
	ret, _, _ := winPdhGetFormattedCounterArray.Call(
		uintptr(hCounter),
		uintptr(PDH_FMT_DOUBLE),
		uintptr(unsafe.Pointer(lpdwBufferSize)),
		uintptr(unsafe.Pointer(lpdwBufferCount)),
		uintptr(unsafe.Pointer(itemBuffer)))

	return uint32(ret)
}

func getCounterArrayData(counter PDH_HCOUNTER) ([]float64, error) {
	var bufSize uint32
	var bufCount uint32

	// Retrieving array data requires two calls, the first which
	// requires an addressable empty buffer, and sets size fields.
	// The second call returns the data.
	initialBuf := make([]PDH_FMT_COUNTERVALUE_ITEM_DOUBLE, 1)
	ret := pdhGetFormattedCounterArrayDouble(counter, &bufSize, &bufCount, &initialBuf[0])
	if ret == PDH_MORE_DATA {
		// we'll likely never get here, but be safe.
		if bufCount > maxQuerySize {
			bufCount = maxQuerySize
		}
		ret = pdhGetFormattedCounterArrayDouble(counter, &bufSize, &bufCount, &counterResults[0])
		if ret == 0 {
			rv := make([]float64, bufCount)
			for i := 0; i < int(bufCount); i++ {
				rv[i] = counterResults[i].FmtValue.DoubleValue
			}
			return rv, nil
		}
	}
	if ret != 0 {
		return nil, fmt.Errorf("getCounterArrayData failed - %d", ret)
	}

	return nil, nil
}

// getProcessImageName returns the name of the process image, as expected by
// the performance counter API.
func getProcessImageName() (name string) {
	name = filepath.Base(os.Args[0])
	name = strings.TrimRight(name, ".exe")
	return
}

// initialize our counters
func initCounters() (err error) {

	processPid = os.Getpid()
	// require an addressible nil pointer
	var source uint16
	if err := pdhOpenQuery(&source, 0, &pcHandle); err != nil {
		return err
	}

	// setup the performance counters, search for all server instances
	name := fmt.Sprintf("%s*", getProcessImageName())
	pidQuery := fmt.Sprintf("\\Process(%s)\\ID Process", name)
	cpuQuery := fmt.Sprintf("\\Process(%s)\\%% Processor Time", name)
	rssQuery := fmt.Sprintf("\\Process(%s)\\Working Set - Private", name)
	vssQuery := fmt.Sprintf("\\Process(%s)\\Virtual Bytes", name)

	if err = pdhAddCounter(pcHandle, pidQuery, 0, &pidCounter); err != nil {
		return err
	}
	if err = pdhAddCounter(pcHandle, cpuQuery, 0, &cpuCounter); err != nil {
		return err
	}
	if err = pdhAddCounter(pcHandle, rssQuery, 0, &rssCounter); err != nil {
		return err
	}
	if err = pdhAddCounter(pcHandle, vssQuery, 0, &vssCounter); err != nil {
		return err
	}

	// prime the counters by collecting once, and sleep to get somewhat
	// useful information the first request.  Counters for the CPU require
	// at least two collect calls.
	if err = pdhCollectQueryData(pcHandle); err != nil {
		return err
	}
	time.Sleep(50)

	return nil
}

// ProcUsage returns process CPU and memory statistics
func ProcUsage(pcpu *float64, rss, vss *int64) error {
	var err error

	// For simplicity, protect the entire call.
	// Most simultaneous requests will immediately return
	// with cached values.
	pcQueryLock.Lock()
	defer pcQueryLock.Unlock()

	// First time through, initialize counters.
	if initialSample {
		if err = initCounters(); err != nil {
			return err
		}
		initialSample = false
	} else if time.Since(lastSampleTime) < (2 * time.Second) {
		// only refresh every two seconds as to minimize impact
		// on the server.
		*pcpu = prevCPU
		*rss = prevRss
		*vss = prevVss
		return nil
	}

	// always save the sample time, even on errors.
	defer func() {
		lastSampleTime = time.Now()
	}()

	// refresh the performance counter data
	if err = pdhCollectQueryData(pcHandle); err != nil {
		return err
	}

	// retrieve the data
	var pidAry, cpuAry, rssAry, vssAry []float64
	if pidAry, err = getCounterArrayData(pidCounter); err != nil {
		return err
	}
	if cpuAry, err = getCounterArrayData(cpuCounter); err != nil {
		return err
	}
	if rssAry, err = getCounterArrayData(rssCounter); err != nil {
		return err
	}
	if vssAry, err = getCounterArrayData(vssCounter); err != nil {
		return err
	}
	// find the index of the entry for this process
	idx := int(-1)
	for i := range pidAry {
		if int(pidAry[i]) == processPid {
			idx = i
			break
		}
	}
	// no pid found...
	if idx < 0 {
		return fmt.Errorf("could not find pid in performance counter results")
	}
	// assign values from the performance counters
	*pcpu = cpuAry[idx]
	*rss = int64(rssAry[idx])
	*vss = int64(vssAry[idx])

	// save off cache values
	prevCPU = *pcpu
	prevRss = *rss
	prevVss = *vss

	return nil
}
