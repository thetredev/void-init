package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

// logPath is where void-init keeps a persistent record of what it did on
// each boot. void-init runs from /etc/rc.local before any syslog daemon
// (e.g. socklog) is started, so there's no /dev/log to write to yet -
// writing directly to a file under /var/log is the closest equivalent
// available this early in boot.
const logPath = "/var/log/void-init.log"

// level is a log severity, ordered least to most severe.
type level int

const (
	levelInfo level = iota
	levelWarn
	levelError
)

func (l level) String() string {
	switch l {
	case levelWarn:
		return "WARN"
	case levelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// logWriter is where log lines are written to in addition to stderr. It is
// nil if logPath couldn't be opened (e.g. /var isn't writable yet), in
// which case logging falls back to stderr only.
var logWriter io.WriteCloser

func init() {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "void-init: warning: open %s: %v (logging to stderr only)\n", logPath, err)
		return
	}
	logWriter = f
}

// closeLog flushes and closes the log file, if one was opened. It should be
// called once, right before the process exits.
func closeLog() {
	if logWriter != nil {
		logWriter.Close()
	}
}

// logf formats and emits one log line at the given level, in a format
// modeled after the classic syslog (RFC3164) line: a timestamp, hostname,
// "program[pid]:", then the level and message. The line is written to
// stderr - which during early boot ends up on the console, since void-init
// runs before any getty/logger takes over - and, best-effort, appended to
// logPath.
func logf(lvl level, format string, args ...any) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "-"
	}

	line := fmt.Sprintf("%s %s void-init[%d]: %s: %s\n",
		time.Now().Format("Jan _2 15:04:05"), hostname, os.Getpid(), lvl, fmt.Sprintf(format, args...))

	fmt.Fprint(os.Stderr, line)
	if logWriter != nil {
		fmt.Fprint(logWriter, line)
	}
}

func logInfo(format string, args ...any)  { logf(levelInfo, format, args...) }
func logWarn(format string, args ...any)  { logf(levelWarn, format, args...) }
func logError(format string, args ...any) { logf(levelError, format, args...) }
