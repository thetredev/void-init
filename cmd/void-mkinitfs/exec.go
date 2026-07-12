package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runCommand runs an external command, logging it first (mirroring
// void-init's own convention of logging every external command before it
// runs), and wraps any failure with the command's combined output.
func runCommand(args ...string) ([]byte, error) {
	logInfo("running: %s", strings.Join(args, " "))

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, output)
	}

	return output, nil
}

// runCommandEnv is runCommand with extra environment variables appended
// to the child's environment (on top of the current process's own).
func runCommandEnv(extra []string, args ...string) ([]byte, error) {
	logInfo("running: %s (%s)", strings.Join(args, " "), strings.Join(extra, " "))

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), extra...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, output)
	}

	return output, nil
}
