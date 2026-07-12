package vlog

import (
	"fmt"
	"os"
	"path/filepath"
)

// maxLogSize is the size threshold, in bytes, at which a Logger's log
// file gets rotated.
const maxLogSize = 50 * 1024 * 1024 // 50 MiB

// maxLogBackups is how many rotated segments are kept alongside the
// active log file (name.log.1 .. name.log.5), oldest evicted first once
// that limit is reached.
const maxLogBackups = 5

// rotatingWriter is an io.WriteCloser over a single log file that rotates
// itself once it grows past maxLogSize: the active file becomes .1, .1
// becomes .2, and so on up to maxLogBackups, with the oldest segment
// discarded. Size is tracked in memory rather than stat'd on every write,
// since it's only ever appended to a file it opened itself.
type rotatingWriter struct {
	path string
	file *os.File
	size int64
}

// openRotatingWriter opens (creating the file and its parent directory if
// necessary) the log file at path, rotating it first if it's already at
// or past maxLogSize.
func openRotatingWriter(path string) (*rotatingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}

	if info, err := os.Stat(path); err == nil && info.Size() >= maxLogSize {
		if err := rotateLogFile(path); err != nil {
			return nil, err
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	return &rotatingWriter{path: path, file: f, size: info.Size()}, nil
}

// Write appends p to the log file, rotating first if p would push the
// file past maxLogSize.
func (w *rotatingWriter) Write(p []byte) (int, error) {
	if w.size > 0 && w.size+int64(len(p)) > maxLogSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)

	return n, err
}

// Close closes the underlying file.
func (w *rotatingWriter) Close() error {
	return w.file.Close()
}

// rotate closes the current file, shifts every existing backup up by one
// slot, and reopens a fresh, empty file at w.path.
func (w *rotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", w.path, err)
	}

	if err := rotateLogFile(w.path); err != nil {
		return err
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", w.path, err)
	}

	w.file = f
	w.size = 0

	return nil
}

// rotateLogFile shifts path's numbered backups up by one slot
// (path.N -> path.N+1, oldest discarded past maxLogBackups) and moves
// path itself to path.1. path does not need to exist yet.
func rotateLogFile(path string) error {
	oldest := fmt.Sprintf("%s.%d", path, maxLogBackups)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", oldest, err)
	}

	for n := maxLogBackups - 1; n >= 1; n-- {
		src := fmt.Sprintf("%s.%d", path, n)
		dst := fmt.Sprintf("%s.%d", path, n+1)

		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rename %s to %s: %w", src, dst, err)
		}
	}

	if err := os.Rename(path, path+".1"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rename %s to %s.1: %w", path, path, err)
	}

	return nil
}
