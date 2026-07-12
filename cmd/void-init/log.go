package main

import "github.com/thetredev/void-init/internal/vlog"

// logPath is where void-init keeps a persistent record of what it did on
// each boot. void-init runs from /etc/rc.local before any syslog daemon
// (e.g. socklog) is started, so there's no /dev/log to write to yet -
// writing directly to a file under /var/log is the closest equivalent
// available this early in boot. It lives under its own directory (rather
// than directly in /var/log) alongside the rotated segments vlog.New
// creates next to it (void-init.log.1, .2, ...).
const logPath = "/var/log/void-init/void-init.log"

var logger = vlog.New("void-init", logPath)

// closeLog flushes and closes the log file, if one was opened. It should
// be called once, right before the process exits.
func closeLog() { logger.Close() }

func logInfo(format string, args ...any)  { logger.Info(format, args...) }
func logWarn(format string, args ...any)  { logger.Warn(format, args...) }
func logError(format string, args ...any) { logger.Error(format, args...) }
