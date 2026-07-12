package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// staticXbpsURL is Void's static xbps tarball for a musl x86_64 build,
// which runs standalone on any Linux host regardless of the host's own
// libc. The static builds track a "latest" alias so this URL doesn't
// need a version number. Per void-mkinitfs.md's "Open items": verify
// this URL/filename against the live static repo before relying on it.
const staticXbpsURL = "https://repo-default.voidlinux.org/static/xbps-static-latest.x86_64-musl.tar.xz"

// xbpsTools are the binaries ensureXbps checks for / installs.
var xbpsTools = []string{"xbps-install", "xbps-reconfigure"}

// localBinDir is where static xbps binaries are installed if they can't
// be found anywhere on PATH. void-mkinitfs runs as root, so this targets
// the same system-wide location as a manual "make install" of locally
// built tools, rather than a per-user directory.
const localBinDir = "/usr/local/bin"

// ensureXbps makes sure xbps-install and xbps-reconfigure are reachable:
// already on PATH, already installed in /usr/local/bin, or downloaded
// into /usr/local/bin from Void's static xbps tarball after asking the
// user for permission.
func ensureXbps() error {
	bin := localBinDir

	// Make a prior install into /usr/local/bin visible to LookPath, in
	// case it isn't already on this process's PATH.
	extendPath(bin)

	var missing []string
	for _, tool := range xbpsTools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	logInfo("%v not found on PATH or in %s", missing, bin)
	if !confirm(fmt.Sprintf("download Void's static xbps tools from %s into %s?", staticXbpsURL, bin)) {
		return fmt.Errorf("%v required but not available", missing)
	}

	if err := downloadStaticXbps(bin); err != nil {
		return fmt.Errorf("download static xbps: %w", err)
	}

	for _, tool := range xbpsTools {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s still not found on PATH after download", tool)
		}
	}

	return nil
}

// extendPath appends dir to the process's PATH environment variable, if
// it isn't already on it, so exec.LookPath picks up binaries placed
// there.
func extendPath(dir string) {
	path := os.Getenv("PATH")
	for _, entry := range filepath.SplitList(path) {
		if entry == dir {
			return
		}
	}
	os.Setenv("PATH", path+string(os.PathListSeparator)+dir)
}

// confirm asks the user a yes/no question on stderr/stdin and reports
// whether they answered yes.
func confirm(question string) bool {
	fmt.Fprintf(os.Stderr, "void-mkinitfs: %s [y/N] ", question)

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// downloadStaticXbps downloads Void's static xbps tarball and extracts
// xbps-install/xbps-reconfigure into destDir. Extraction shells out to
// tar, since the tarball is xz-compressed and the standard library has
// no xz decoder.
func downloadStaticXbps(destDir string) error {
	if _, err := exec.LookPath("tar"); err != nil {
		return fmt.Errorf("tar not found on PATH (needed to extract the static xbps tarball): %w", err)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", destDir, err)
	}

	tmpFile, err := os.CreateTemp("", "void-mkinitfs-xbps-*.tar.xz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	logInfo("downloading %s", staticXbpsURL)

	if err := download(staticXbpsURL, tmpFile); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}

	extractDir, err := os.MkdirTemp("", "void-mkinitfs-xbps-extract")
	if err != nil {
		return fmt.Errorf("create extract dir: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if _, err := runCommand("tar", "-xJf", tmpPath, "-C", extractDir); err != nil {
		return err
	}

	return installExtractedXbps(extractDir, destDir)
}

// download fetches url and writes its body to w.
func download(url string, w io.Writer) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}

	return nil
}

// installExtractedXbps walks extractDir for the xbps binaries this tool
// needs and copies them into destDir, since the static tarball's
// internal directory layout isn't assumed to be stable across releases.
func installExtractedXbps(extractDir, destDir string) error {
	wanted := make(map[string]bool, len(xbpsTools))
	for _, tool := range xbpsTools {
		wanted[tool] = true
	}

	found := map[string]string{}

	err := filepath.WalkDir(extractDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if wanted[d.Name()] {
			found[d.Name()] = path
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", extractDir, err)
	}

	for _, tool := range xbpsTools {
		src, ok := found[tool]
		if !ok {
			return fmt.Errorf("%s not found in downloaded tarball", tool)
		}

		dst := filepath.Join(destDir, tool)
		if err := copyFile(src, dst, 0o755); err != nil {
			return err
		}
		logInfo("installed %s", dst)
	}

	return nil
}
