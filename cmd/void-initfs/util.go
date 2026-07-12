package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

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
