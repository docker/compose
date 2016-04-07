// +build arm64,linux

package archutils

// #include <sys/epoll.h>
/*
int EpollCreate1(int flag) {
        return epoll_create1(0);
}

int EpollCtl(int efd, int op,int sfd, int Events, int Fd) {
        struct epoll_event event;
        event.events = Events;
        event.data.fd = Fd;

        return epoll_ctl(efd,op,sfd,&event);
}

typedef struct Event{
        uint32_t events;
        int fd;
};

struct epoll_event events[128];
int run_epoll_wait(int fd, struct Event *event) {
        int n, i;
        n = epoll_wait(fd, events, 128, -1);
        for (i = 0; i < n; i++) {
                event[i].events = events[i].events;
                event[i].fd = events[i].data.fd;
        }
        return n;
}
*/
import "C"

import (
	"fmt"
	"syscall"
	"unsafe"
)

func EpollCreate1(flag int) (int, error) {
	fd := int(C.EpollCreate1(0))
	if fd < 0 {
		return fd, fmt.Errorf("failed to create epoll, errno is %d", fd)
	}
	return fd, nil
}

func EpollCtl(epfd int, op int, fd int, event *syscall.EpollEvent) error {
	errno := C.EpollCtl(C.int(epfd), C.int(syscall.EPOLL_CTL_ADD), C.int(fd), C.int(event.Events), C.int(event.Fd))
	if errno < 0 {
		return fmt.Errorf("Failed to ctl epoll")
	}
	return nil
}

func EpollWait(epfd int, events []syscall.EpollEvent, msec int) (int, error) {
	var c_events [128]C.struct_Event
	n := int(C.run_epoll_wait(C.int(epfd), (*C.struct_Event)(unsafe.Pointer(&c_events))))
	if n < 0 {
		return int(n), fmt.Errorf("Failed to wait epoll")
	}
	for i := 0; i < n; i++ {
		events[i].Fd = int32(c_events[i].fd)
		events[i].Events = uint32(c_events[i].events)
	}
	return int(n), nil
}
