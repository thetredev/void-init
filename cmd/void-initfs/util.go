package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// userConfigMarker delimits the managed portion of a void-initfs-written
// file from a trailing section left for the user, mirroring void-init's own
// marker (see userConfigMarker in cmd/void-init/fsutil.go). Anything at or
// after this marker in a file already present in root is preserved verbatim
// across reruns of void-initfs against the same rootfs.
const userConfigMarker = "#void-init: user config starts here"

// writeManagedFile writes rendered to path, preserving whatever the user
// has appended after userConfigMarker in the file that's already there (if
// any). rendered itself is expected to end with the marker, followed by
// nothing, i.e. the default (empty) user section for a fresh image.
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

	return writeFile(path, header+userSection, perm)
}

// writeFile writes content to path with the given permissions, ending
// with exactly one trailing newline - mirroring void-init's own
// file-writing convention (see withSingleTrailingNewline in
// cmd/void-init/fsutil.go). perm is applied with an explicit chmod, since
// os.WriteFile only honors it when creating the file (and masks it with
// the umask) - image content should get exactly the stated mode
// regardless of what already existed or how the build host's umask is
// set.
func writeFile(path, content string, perm os.FileMode) error {
	trimmed := strings.TrimRight(content, "\n\t ") + "\n"
	if err := os.WriteFile(path, []byte(trimmed), perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

// copyFile copies src to dst with the given permissions. perm is applied
// with an explicit chmod for the same reason as in writeFile: O_CREATE's
// mode only takes effect when dst doesn't already exist, and is masked
// by the umask when it does take effect.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}

	if err := out.Chmod(perm); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}

	return nil
}
