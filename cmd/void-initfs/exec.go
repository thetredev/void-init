package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// runCommand runs an external command, logging it first (mirroring
// void-init's own convention of logging every external command before it
// runs).
func runCommand(args ...string) ([]byte, error) {
	logInfo("running: %s", strings.Join(args, " "))
	return runCommandEnv(nil, args...)
}

// runCommandEnv is runCommand with extra environment variables appended to
// the child's environment (on top of the current process's own). Stdout
// and stderr are streamed to void-initfs's own stdout/stderr live, the
// same way they'd appear running the command by hand - important for
// long-running, chatty commands like xbps-install, whose progress output
// would otherwise be invisible until the command finished (or silently
// swallowed entirely on success). The combined output is also captured
// into a buffer, so a failure can still be wrapped with exactly what the
// command printed.
func runCommandEnv(extra []string, args ...string) ([]byte, error) {
	if len(extra) > 0 {
		logInfo("running: %s (%s)", strings.Join(args, " "), strings.Join(extra, " "))
	}

	cmd := exec.Command(args[0], args[1:]...)
	if len(extra) > 0 {
		cmd.Env = append(os.Environ(), extra...)
	}

	var output bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)

	if err := cmd.Run(); err != nil {
		return output.Bytes(), fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, output.Bytes())
	}

	return output.Bytes(), nil
}
