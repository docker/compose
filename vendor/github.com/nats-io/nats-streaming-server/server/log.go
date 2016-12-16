// Copyright 2016 Apcera Inc. All rights reserved.

package server

import (
	"github.com/nats-io/gnatsd/logger"
	natsd "github.com/nats-io/gnatsd/server"
	"os"
	"sync"
	"sync/atomic"
)

// Logging in STAN
//
// The STAN logger is an instance of a NATS logger, (basically duplicated
// from the NATS server code), and is passed into the NATS server.
//
// A note on Debugf and Tracef:  These will be enabled within the log if
// either STAN or the NATS server enables them.  However, STAN will only
// trace/debug if the local STAN debug/trace flags are set.  NATS will do
// the same with it's logger flags.  This enables us to use the same logger,
// but differentiate between STAN and NATS debug/trace.
//
// All logging functions are fully implemented (versus calling into the NATS
// server) in case STAN is decoupled from the NATS server.

// Package globals for performance checks
var trace int32
var debug int32

// The STAN logger, encapsulates a NATS logger
var stanLog = struct {
	sync.Mutex
	logger natsd.Logger
}{}

// ConfigureLogger configures logging for STAN and the embedded NATS server
// based on options passed.
func ConfigureLogger(stanOpts *Options, natsOpts *natsd.Options) {

	var s *natsd.Server
	var newLogger natsd.Logger

	sOpts := stanOpts
	nOpts := natsOpts

	if sOpts == nil {
		sOpts = GetDefaultOptions()
	}
	if nOpts == nil {
		nOpts = &natsd.Options{}
	}

	enableDebug := nOpts.Debug || sOpts.Debug
	enableTrace := nOpts.Trace || sOpts.Trace

	if nOpts.LogFile != "" {
		newLogger = logger.NewFileLogger(nOpts.LogFile, nOpts.Logtime, enableDebug, sOpts.Trace, true)
	} else if nOpts.RemoteSyslog != "" {
		newLogger = logger.NewRemoteSysLogger(nOpts.RemoteSyslog, sOpts.Debug, sOpts.Trace)
	} else if nOpts.Syslog {
		newLogger = logger.NewSysLogger(sOpts.Debug, sOpts.Trace)
	} else {
		colors := true
		// Check to see if stderr is being redirected and if so turn off color
		// Also turn off colors if we're running on Windows where os.Stderr.Stat() returns an invalid handle-error
		stat, err := os.Stderr.Stat()
		if err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
			colors = false
		}
		newLogger = logger.NewStdLogger(nOpts.Logtime, enableDebug, enableTrace, colors, true)
	}
	if sOpts.Debug {
		atomic.StoreInt32(&debug, 1)
	}
	if sOpts.Trace {
		atomic.StoreInt32(&trace, 1)
	}

	// The NATS server will use the STAN logger
	s.SetLogger(newLogger, nOpts.Debug, nOpts.Trace)

	stanLog.Lock()
	stanLog.logger = newLogger
	stanLog.Unlock()
}

// RemoveLogger clears the logger instance and debug/trace flags.
// Used for testing.
func RemoveLogger() {
	var s *natsd.Server

	atomic.StoreInt32(&trace, 0)
	atomic.StoreInt32(&debug, 0)

	stanLog.Lock()
	stanLog.logger = nil
	stanLog.Unlock()

	s.SetLogger(nil, false, false)
}

// Noticef logs a notice statement
func Noticef(format string, v ...interface{}) {
	executeLogCall(func(log natsd.Logger, format string, v ...interface{}) {
		log.Noticef(format, v...)
	}, format, v...)
}

// Errorf logs an error
func Errorf(format string, v ...interface{}) {
	executeLogCall(func(log natsd.Logger, format string, v ...interface{}) {
		log.Errorf(format, v...)
	}, format, v...)
}

// Fatalf logs a fatal error
func Fatalf(format string, v ...interface{}) {
	executeLogCall(func(log natsd.Logger, format string, v ...interface{}) {
		log.Fatalf(format, v...)
	}, format, v...)
}

// Debugf logs a debug statement
func Debugf(format string, v ...interface{}) {
	if atomic.LoadInt32(&debug) != 0 {
		executeLogCall(func(log natsd.Logger, format string, v ...interface{}) {
			log.Debugf(format, v...)
		}, format, v...)
	}
}

// Tracef logs a trace statement
func Tracef(format string, v ...interface{}) {
	if atomic.LoadInt32(&trace) != 0 {
		executeLogCall(func(logger natsd.Logger, format string, v ...interface{}) {
			logger.Tracef(format, v...)
		}, format, v...)
	}
}

func executeLogCall(f func(logger natsd.Logger, format string, v ...interface{}), format string, args ...interface{}) {
	stanLog.Lock()
	defer stanLog.Unlock()
	if stanLog.logger == nil {
		return
	}
	f(stanLog.logger, format, args...)
}
