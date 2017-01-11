// +build !windows
// Copyright 2012-2016 Apcera Inc. All rights reserved.

package server

import (
	"os"
	"os/signal"
	"syscall"
)

// Signal Handling
func (s *Server) handleSignals() {
	if s.opts.NoSigs {
		return
	}
	c := make(chan os.Signal, 1)

	signal.Notify(c, syscall.SIGINT, syscall.SIGUSR1)

	go func() {
		for sig := range c {
			Debugf("Trapped %q signal", sig)
			switch sig {
			case syscall.SIGINT:
				Noticef("Server Exiting..")
				os.Exit(0)
			case syscall.SIGUSR1:
				// File log re-open for rotating file logs.
				s.ReOpenLogFile()
			}
		}
	}()
}
