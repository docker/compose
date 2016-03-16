// Copyright (c) 2012 VMware, Inc.

package sigar

// #include <stdlib.h>
// #include <windows.h>
import "C"

import (
	"fmt"
	"unsafe"
)

func init() {
}

func (self *LoadAverage) Get() error {
	return nil
}

func (self *Uptime) Get() error {
	return nil
}

func (self *Mem) Get() error {
	var statex C.MEMORYSTATUSEX
	statex.dwLength = C.DWORD(unsafe.Sizeof(statex))

	succeeded := C.GlobalMemoryStatusEx(&statex)
	if succeeded == C.FALSE {
		lastError := C.GetLastError()
		return fmt.Errorf("GlobalMemoryStatusEx failed with error: %d", int(lastError))
	}

	self.Total = uint64(statex.ullTotalPhys)
	return nil
}

func (self *Swap) Get() error {
	return notImplemented()
}

func (self *Cpu) Get() error {
	return notImplemented()
}

func (self *CpuList) Get() error {
	return notImplemented()
}

func (self *FileSystemList) Get() error {
	return notImplemented()
}

func (self *ProcList) Get() error {
	return notImplemented()
}

func (self *ProcState) Get(pid int) error {
	return notImplemented()
}

func (self *ProcMem) Get(pid int) error {
	return notImplemented()
}

func (self *ProcTime) Get(pid int) error {
	return notImplemented()
}

func (self *ProcArgs) Get(pid int) error {
	return notImplemented()
}

func (self *ProcExe) Get(pid int) error {
	return notImplemented()
}

func (self *FileSystemUsage) Get(path string) error {
	var availableBytes C.ULARGE_INTEGER
	var totalBytes C.ULARGE_INTEGER
	var totalFreeBytes C.ULARGE_INTEGER

	pathChars := C.CString(path)
	defer C.free(unsafe.Pointer(pathChars))

	succeeded := C.GetDiskFreeSpaceEx((*C.CHAR)(pathChars), &availableBytes, &totalBytes, &totalFreeBytes)
	if succeeded == C.FALSE {
		lastError := C.GetLastError()
		return fmt.Errorf("GetDiskFreeSpaceEx failed with error: %d", int(lastError))
	}

	self.Total = *(*uint64)(unsafe.Pointer(&totalBytes))
	return nil
}

func notImplemented() error {
	panic("Not Implemented")
	return nil
}
