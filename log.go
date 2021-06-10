// mgo - MongoDB driver for Go
//
// Copyright (c) 2010-2012 - Gustavo Niemeyer <gustavo@niemeyer.net>
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
// ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package mgo

import (
	"bytes"
	"fmt"
	"github.com/getsentry/raven-go"
	shellLog "log"
	"os"
	shellDebug "runtime/debug"
	"sync"
	"unicode/utf8"
)

const (
	max_log_output = 256

	LOG_DEBUG = 0
	LOG_INFO  = 1
	LOG_WARN  = 2
	LOG_ERROR = 3
	LOG_FATAL = 4

	error_str = "\033[031;1m[ERROR]\033[031;0m"
)

// ---------------------------------------------------------------------------
// Logging integration.

// LogLogger avoid importing the log type information unnecessarily.  There's a small cost
// associated with using an interface rather than the type.  Depending on how
// often the logger is plugged in, it would be worth using the type instead.
type logLogger interface {
	Output(calldepth int, s string) error
}

var (
	_prefix          string
	_openSentry      bool
	globalLogLevel   int
	globalLoggerFunc func(string) *shellLog.Logger
	globalFormatLogf func(int, int, string, ...interface{}) string

	globalLogger logLogger
	globalDebug  bool
	globalMutex  sync.Mutex
)

// RACE WARNING: There are known data races when logging, which are manually
// silenced when the race detector is in use. These data races won't be
// observed in typical use, because logging is supposed to be set up once when
// the application starts. Having raceDetector as a constant, the compiler
// should elide the locks altogether in actual use.

// SetLogger specify the *log.Logger object where log messages should be sent to.
func SetLogger(logger logLogger) {
	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}
	globalLogger = logger
}

func SetLoggerFunc(serverPrefix string, sentryOpen bool, logLevel int, loggerFunc func(string) *shellLog.Logger, formatLogf func(int, int, string, ...interface{}) string) {
	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}
	_prefix = serverPrefix
	_openSentry = sentryOpen
	globalLoggerFunc = loggerFunc
	globalLogLevel = logLevel
	globalFormatLogf = formatLogf
}

func getLogger(typ string) logLogger {
	if globalLogger != nil {
		return globalLogger
	}

	if typ != "" && globalLoggerFunc != nil {
		return globalLoggerFunc(typ)
	}

	return nil
}

// SetDebug enable the delivery of debug messages to the logger.  Only meaningful
// if a logger is also set.
func SetDebug(debug bool) {
	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}
	globalDebug = debug
}

func writeLogFile(writeLogLevel int, logger logLogger, logStr string) {
	if globalFormatLogf == nil {
		return
	}
	flagLogStr := globalFormatLogf(writeLogLevel, 4, "%s", logStr)

	if writeLogLevel >= LOG_ERROR && _openSentry {
		raven.CaptureMessage(fmt.Sprintf("%s%s", _prefix, flagLogStr[len(error_str):]), nil)
	}
	if logger == nil {
		// 如果没有logger则写到标准输出
		shellLog.Println(flagLogStr)
		return
	} else {
		logger.Output(2, flagLogStr)
	}
}

func log(v ...interface{}) {
	if globalLogLevel > LOG_INFO {
		return
	}

	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}

	writeLogFile(LOG_INFO, getLogger(""), fmt.Sprint(v...))
}

func logln(v ...interface{}) {
	if globalLogLevel > LOG_INFO {
		return
	}

	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}

	writeLogFile(LOG_INFO, getLogger(""), fmt.Sprintln(v...))
}

func logf(format string, v ...interface{}) {
	if globalLogLevel > LOG_INFO {
		return
	}

	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}

	writeLogFile(LOG_INFO, getLogger(""), fmt.Sprintf(format, v...))
}

func debug(v ...interface{}) {
	if globalLogLevel > LOG_DEBUG {
		return
	}
	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}

	logStr := fmt.Sprint(v...)
	if utf8.RuneCountInString(logStr) <= max_log_output {
		writeLogFile(LOG_DEBUG, getLogger(""), logStr)
	}
}

func debugln(v ...interface{}) {
	if globalLogLevel > LOG_DEBUG {
		return
	}

	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}

	logStr := fmt.Sprintln(v...)
	if utf8.RuneCountInString(logStr) <= max_log_output {
		writeLogFile(LOG_DEBUG, getLogger(""), logStr)
	}
}

func debugf(format string, v ...interface{}) {
	if globalLogLevel > LOG_DEBUG {
		return
	}

	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}

	logStr := fmt.Sprintf(format, v...)
	if utf8.RuneCountInString(logStr) <= max_log_output {
		writeLogFile(LOG_DEBUG, getLogger(""), logStr)
	}
}

func errorln(v ...interface{}) {
	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}

	writeLogFile(LOG_ERROR, getLogger("error"), fmt.Sprintln(v...))
}

func errorf(format string, v ...interface{}) {
	if raceDetector {
		globalMutex.Lock()
		defer globalMutex.Unlock()
	}

	writeLogFile(LOG_ERROR, getLogger("error"), fmt.Sprintf(format, v...))
}

func backTrace(name string) {
	errorln("goroutine[%s] is exiting...\n", name)
	buf := bytes.NewBuffer(shellDebug.Stack())
	fmt.Fprintf(os.Stderr, buf.String())
	errorln(buf.String())
}
