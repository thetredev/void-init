package main

import "github.com/thetredev/void-init/internal/vlog"

// void-initfs is an interactive build tool run on the host machine, not
// a boot-time system record - it logs to stderr only, no file sink.
var logger = vlog.New("void-initfs", "")

func logInfo(format string, args ...any)  { logger.Info(format, args...) }
func logWarn(format string, args ...any)  { logger.Warn(format, args...) }
func logError(format string, args ...any) { logger.Error(format, args...) }
