package dockerclassic

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func init() {
	if err := killSubProcessesOnClose(); err != nil {
		fmt.Println("failed to create job:", err)
	}
}

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
)

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation struct {
		PerProcessUserTimeLimit uint64
		PerJobUserTimeLimit     uint64
		LimitFlags              uint32
		MinimumWorkingSetSize   uintptr
		MaximumWorkingSetSize   uintptr
		ActiveProcessLimit      uint32
		Affinity                uintptr
		PriorityClass           uint32
		SchedulingClass         uint32
	}
	IoInfo struct {
		ReadOperationCount  uint64
		WriteOperationCount uint64
		OtherOperationCount uint64
		ReadTransferCount   uint64
		WriteTransferCount  uint64
		OtherTransferCount  uint64
	}
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// killSubProcessesOnClose will ensure on windows that all child processes of the current process are killed if parent is killed.
func killSubProcessesOnClose() error {
	job, err := createJobObject()
	if err != nil {
		return err
	}
	info := jobObjectExtendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = 0x2000
	if err := setInformationJobObject(job, info); err != nil {
		_ = syscall.CloseHandle(job)
		return err
	}
	proc, err := syscall.GetCurrentProcess()
	if err != nil {
		_ = syscall.CloseHandle(job)
		return err
	}
	if err := assignProcessToJobObject(job, proc); err != nil {
		_ = syscall.CloseHandle(job)
		return err
	}
	return nil
}

func createJobObject() (syscall.Handle, error) {
	res, _, err := kernel32.NewProc("CreateJobObjectW").Call(uintptr(unsafe.Pointer(nil)), uintptr(unsafe.Pointer(nil)))
	if res == 0 {
		return syscall.InvalidHandle, os.NewSyscallError("CreateJobObject", err)
	}
	return syscall.Handle(res), nil
}

func setInformationJobObject(job syscall.Handle, info jobObjectExtendedLimitInformation) error {
	infoClass := uint32(9)
	res, _, err := kernel32.NewProc("SetInformationJobObject").Call(uintptr(job), uintptr(infoClass), uintptr(unsafe.Pointer(&info)), uintptr(uint32(unsafe.Sizeof(info))))
	if res == 0 {
		return os.NewSyscallError("SetInformationJobObject", err)
	}
	return nil
}

func assignProcessToJobObject(job syscall.Handle, process syscall.Handle) error {
	res, _, err := kernel32.NewProc("AssignProcessToJobObject").Call(uintptr(job), uintptr(process))
	if res == 0 {
		return os.NewSyscallError("AssignProcessToJobObject", err)
	}
	return nil
}
