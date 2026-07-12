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
// cmd/void-init/fsutil.go).
func writeFile(path, content string, perm os.FileMode) error {
	trimmed := strings.TrimRight(content, "\n\t ") + "\n"
	if err := os.WriteFile(path, []byte(trimmed), perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// copyFile copies src to dst with the given permissions.
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

	return nil
}
