package main

import (
	"fmt"
	"os"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd"
	"github.com/docker/containerd/api/grpc/server"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/supervisor"
	"github.com/docker/docker/pkg/listeners"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func startServer(protocol, address string, sv *supervisor.Supervisor) (*grpc.Server, error) {
	sockets, err := listeners.Init(protocol, address, "", nil)
	if err != nil {
		return nil, err
	}
	if len(sockets) != 1 {
		return nil, fmt.Errorf("incorrect number of listeners")
	}
	l := sockets[0]
	s := grpc.NewServer()
	types.RegisterAPIServer(s, server.NewServer(sv))
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	go func() {
		logrus.Debugf("containerd: grpc api on %s", address)
		if err := s.Serve(l); err != nil {
			logrus.WithField("error", err).Fatal("containerd: serve grpc")
		}
	}()
	return s, nil
}

// getDefaultID returns the hostname for the instance host
func getDefaultID() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return hostname
}

func checkLimits() error {
	var l syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &l); err != nil {
		return err
	}
	if l.Cur <= minRlimit {
		logrus.WithFields(logrus.Fields{
			"current": l.Cur,
			"max":     l.Max,
		}).Warn("containerd: low RLIMIT_NOFILE changing to max")
		l.Cur = l.Max
		return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &l)
	}
	return nil
}

func getVersion() string {
	if containerd.GitCommit != "" {
		return fmt.Sprintf("%s commit: %s", containerd.Version, containerd.GitCommit)
	}
	return containerd.Version
}
