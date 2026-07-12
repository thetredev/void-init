// Package vlog implements the leveled, syslog-style logging shared by
// void-init and void-mkinitfs.
package vlog

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Level is a log severity.
type Level int

const (
	LevelInfo Level = iota
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Logger writes leveled log lines to stderr and, optionally, appends the
// same lines to a file.
type Logger struct {
	program string
	file    io.WriteCloser
}

// New creates a Logger for program. If logPath is non-empty, it
// best-effort opens/appends that file as an additional sink; if it can't
// be opened, New prints a warning to stderr and falls back to stderr
// only. Pass "" for logPath to always log to stderr only.
func New(program, logPath string) *Logger {
	l := &Logger{program: program}

	if logPath == "" {
		return l
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: warning: open %s: %v (logging to stderr only)\n", program, logPath, err)
		return l
	}

	l.file = f
	return l
}

// Close closes the log file, if one was opened. Call once, right before
// the process exits.
func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}

// logf formats and emits one log line at the given level, in a format
// modeled after the classic syslog (RFC3164) line: a timestamp, hostname,
// "program[pid]:", then the level and message.
func (l *Logger) logf(lvl Level, format string, args ...any) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "-"
	}

	line := fmt.Sprintf("%s %s %s[%d]: %s: %s\n",
		time.Now().Format("Jan _2 15:04:05"), hostname, l.program, os.Getpid(), lvl, fmt.Sprintf(format, args...))

	fmt.Fprint(os.Stderr, line)
	if l.file != nil {
		fmt.Fprint(l.file, line)
	}
}

func (l *Logger) Info(format string, args ...any)  { l.logf(LevelInfo, format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.logf(LevelWarn, format, args...) }
func (l *Logger) Error(format string, args ...any) { l.logf(LevelError, format, args...) }
