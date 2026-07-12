package main

import (
	"fmt"
	"os"
	"strings"
)

// userConfigMarker delimits the managed portion of a void-init-generated
// file from a trailing section left for the user. Anything at or after this
// marker in an existing file is preserved verbatim across reruns.
const userConfigMarker = "#void-init: user config starts here"

// writeManagedFile writes rendered to path, preserving whatever the user
// has appended after userConfigMarker in the file that's already there (if
// any). rendered itself is expected to end with the marker, followed by
// nothing, i.e. the default (empty) user section for a fresh install.
func writeManagedFile(path, rendered string, perm os.FileMode) error {
	header := rendered
	if idx := strings.Index(rendered, userConfigMarker); idx >= 0 {
		header = rendered[:idx+len(userConfigMarker)]
	}

	userSection := ""
	if data, err := os.ReadFile(path); err == nil {
		if idx := strings.Index(string(data), userConfigMarker); idx >= 0 {
			userSection = string(data)[idx+len(userConfigMarker):]
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	if err := os.WriteFile(path, []byte(withSingleTrailingNewline(header+userSection)), perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// withSingleTrailingNewline trims any trailing whitespace/newlines from s
// and appends exactly one "\n", so files void-init writes always end
// cleanly with a single newline.
func withSingleTrailingNewline(s string) string {
	return strings.TrimRight(s, "\n\t ") + "\n"
}
