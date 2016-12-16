// Copyright 2012-2014 Apcera Inc. All rights reserved.
package logger

import (
	"fmt"
	"log"
	"os"
)

type SysLogger struct {
	writer *log.Logger
	debug  bool
	trace  bool
}

func NewSysLogger(debug, trace bool) *SysLogger {
	w := log.New(os.Stdout, "gnatsd", log.LstdFlags)

	return &SysLogger{
		writer: w,
		debug:  debug,
		trace:  trace,
	}
}

func NewRemoteSysLogger(fqn string, debug, trace bool) *SysLogger {
	return NewSysLogger(debug, trace)
}

func (l *SysLogger) Noticef(format string, v ...interface{}) {
	l.writer.Println("NOTICE", fmt.Sprintf(format, v...))
}

func (l *SysLogger) Fatalf(format string, v ...interface{}) {
	l.writer.Println("CRITICAL", fmt.Sprintf(format, v...))
}

func (l *SysLogger) Errorf(format string, v ...interface{}) {
	l.writer.Println("ERROR", fmt.Sprintf(format, v...))
}

func (l *SysLogger) Debugf(format string, v ...interface{}) {
	if l.debug {
		l.writer.Println("DEBUG", fmt.Sprintf(format, v...))
	}
}

func (l *SysLogger) Tracef(format string, v ...interface{}) {
	if l.trace {
		l.writer.Println("NOTICE", fmt.Sprintf(format, v...))
	}
}
