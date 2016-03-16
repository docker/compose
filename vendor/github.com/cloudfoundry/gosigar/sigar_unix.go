// Copyright (c) 2012 VMware, Inc.

// +build darwin freebsd linux netbsd openbsd

package sigar

import "syscall"

func (self *FileSystemUsage) Get(path string) error {
	stat := syscall.Statfs_t{}
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return err
	}

	bsize := stat.Bsize / 512

	self.Total = (uint64(stat.Blocks) * uint64(bsize)) >> 1
	self.Free = (uint64(stat.Bfree) * uint64(bsize)) >> 1
	self.Avail = (uint64(stat.Bavail) * uint64(bsize)) >> 1
	self.Used = self.Total - self.Free
	self.Files = stat.Files
	self.FreeFiles = stat.Ffree

	return nil
}
