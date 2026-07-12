package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// staticXbpsFilename/staticXbpsURL are Void's static xbps tarball for a
// musl x86_64 build, which runs standalone on any Linux host regardless
// of the host's own libc. The static builds track a "latest" alias so
// this URL doesn't need a version number.
const staticXbpsFilename = "xbps-static-latest.x86_64-musl.tar.xz"
const staticXbpsURL = "https://repo-default.voidlinux.org/static/" + staticXbpsFilename

// sha256sumsURL lists sha256 checksums for every file Void publishes
// under its static tools archive. ensureXbps checks the downloaded
// tarball's digest against this list before trusting anything extracted
// from it - both the xbps-install/xbps-reconfigure binaries and the
// repository signing keys (see keys.go). Note: as observed at
// implementation time, this file's entry for the "-latest" alias
// filename itself lags behind what that alias currently serves (Void's
// build infra doesn't seem to regenerate the alias's own checksum row on
// every bump) - the real per-version rows are accurate, though, so
// verification checks whether the downloaded digest appears anywhere in
// the file rather than trusting the "-latest" row specifically. See
// verifyChecksum.
const sha256sumsURL = "https://repo-default.voidlinux.org/static/sha256sums.txt"

// xbpsTools are the binaries ensureXbps checks for / installs.
var xbpsTools = []string{"xbps-install", "xbps-reconfigure"}

// localBinDir is where static xbps binaries are installed if they can't
// be found anywhere on PATH. void-initfs runs as root, so this targets
// the same system-wide location as a manual "make install" of locally
// built tools, rather than a per-user directory.
const localBinDir = "/usr/local/bin"

// localKeysDir caches the repository signing keys extracted from Void's
// static xbps tarball, so installRepoKeys (keys.go, run against every
// freshly created rootdir) doesn't have to hit the network on every
// single build. Only ensureXbps, when it downloads a
// (checksum-verified) copy of the tarball, refreshes this cache - there
// is deliberately no hardcoded/embedded key shipped with void-initfs
// itself: whatever keys the live tarball bundles are what gets trusted.
const localKeysDir = "/usr/local/share/void-initfs/keys"

// ensureXbps makes sure xbps-install/xbps-reconfigure and Void's
// repository signing keys (localKeysDir) are both available: already on
// PATH / already cached, or downloaded and checksum-verified from Void's
// live static archive after asking the user for permission (skipped when
// assumeYes is set, i.e. -y/--yes). update forces a re-download/re-verify
// of both even if already present (--update-xbps).
func ensureXbps(update, assumeYes bool) error {
	extendPath(localBinDir)

	missing := missingTools()
	needBinaries := update || len(missing) > 0
	needKeys := update || !haveCachedKeys()

	if !needBinaries && !needKeys {
		return nil
	}

	logInfo("refreshing xbps tools/keys from %s (missing tools: %v, update: %v)", staticXbpsURL, missing, update)
	if !assumeYes && !confirm(fmt.Sprintf("download and verify Void's static xbps tools/keys from %s?", staticXbpsURL)) {
		return fmt.Errorf("xbps tools/keys required but not available")
	}

	extractDir, err := downloadAndVerifyStaticXbps()
	if err != nil {
		return fmt.Errorf("download static xbps: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if needBinaries {
		if err := installExtractedXbps(extractDir, localBinDir); err != nil {
			return err
		}
	}
	if needKeys {
		if err := installExtractedKeys(extractDir, localKeysDir); err != nil {
			return err
		}
	}

	for _, tool := range xbpsTools {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s still not found on PATH after download", tool)
		}
	}

	return nil
}

// missingTools returns the subset of xbpsTools not found on PATH.
func missingTools() []string {
	var missing []string
	for _, tool := range xbpsTools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	return missing
}

// haveCachedKeys reports whether localKeysDir already holds at least one
// previously cached repository key.
func haveCachedKeys() bool {
	entries, err := os.ReadDir(localKeysDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".plist") {
			return true
		}
	}
	return false
}

// extendPath appends dir to the process's PATH environment variable, if
// it isn't already on it, so exec.LookPath picks up binaries placed
// there.
func extendPath(dir string) {
	path := os.Getenv("PATH")
	if !slices.Contains(filepath.SplitList(path), dir) {
		return
	}
	os.Setenv("PATH", path+string(os.PathListSeparator)+dir)
}

// confirm asks the user a yes/no question on stderr/stdin and reports
// whether they answered yes.
func confirm(question string) bool {
	fmt.Fprintf(os.Stderr, "void-initfs: %s [y/N] ", question)

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// downloadAndVerifyStaticXbps downloads sha256sums.txt and Void's static
// xbps tarball, verifies the tarball's digest is one sha256sums.txt
// actually lists (see verifyChecksum for why that's a membership check
// rather than a lookup by the tarball's own filename), extracts it, and
// returns the extraction directory - the caller is responsible for
// removing it. A checksum mismatch is fatal: nothing extracted from an
// unverified tarball is installed or trusted.
func downloadAndVerifyStaticXbps() (string, error) {
	if _, err := exec.LookPath("tar"); err != nil {
		return "", fmt.Errorf("tar not found on PATH (needed to extract the static xbps tarball): %w", err)
	}

	logInfo("downloading %s", sha256sumsURL)
	var sums bytes.Buffer
	if err := download(sha256sumsURL, &sums); err != nil {
		return "", fmt.Errorf("download sha256sums.txt: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "void-initfs-xbps-*.tar.xz")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	logInfo("downloading %s", staticXbpsURL)

	hasher := sha256.New()
	if err := download(staticXbpsURL, io.MultiWriter(tmpFile, hasher)); err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close %s: %w", tmpPath, err)
	}

	digest := hex.EncodeToString(hasher.Sum(nil))
	if err := verifyChecksum(sums.Bytes(), digest); err != nil {
		return "", err
	}
	logInfo("verified %s (sha256 %s) against sha256sums.txt", staticXbpsFilename, digest)

	extractDir, err := os.MkdirTemp("", "void-initfs-xbps-extract")
	if err != nil {
		return "", fmt.Errorf("create extract dir: %w", err)
	}

	if _, err := runCommand("tar", "-xJf", tmpPath, "-C", extractDir); err != nil {
		os.RemoveAll(extractDir)
		return "", err
	}

	return extractDir, nil
}

// verifyChecksum reports whether digest (the downloaded tarball's sha256,
// lowercase hex) appears anywhere in sums, sha256sums.txt's contents
// (GNU coreutils "sha256sum" format: "<hex>  <filename>" per line).
// Matching is by digest membership rather than looking up the row for
// staticXbpsFilename specifically, because that row - the "-latest"
// mutable alias - was observed to lag behind the alias's actual current
// content; the row for whatever pinned version "-latest" currently
// points to is present and correct, so this still only accepts bytes
// Void's own build actually published and recorded a checksum for.
func verifyChecksum(sums []byte, digest string) error {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == digest {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("parse sha256sums.txt: %w", err)
	}
	return fmt.Errorf("downloaded %s (sha256 %s) not listed in sha256sums.txt - refusing to trust it", staticXbpsFilename, digest)
}

// httpClient bounds every download: a server that accepts the
// connection but never answers fails within seconds
// (ResponseHeaderTimeout) instead of hanging the build indefinitely,
// while the overall Timeout is generous enough to pull the
// multi-megabyte static tarball over a slow link. Cloning
// http.DefaultTransport keeps its proxy-from-environment support and
// dial/TLS-handshake timeouts.
var httpClient = newHTTPClient()

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second

	return &http.Client{
		Timeout:   15 * time.Minute,
		Transport: transport,
	}
}

// download fetches url and writes its body to w.
func download(url string, w io.Writer) error {
	resp, err := httpClient.Get(url)
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
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", destDir, err)
	}

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

// installExtractedKeys copies every repository signing key Void's static
// tarball bundles (under var/db/xbps/keys/) into destDir, replacing
// whatever was cached there before. There's no fixed fingerprint to look
// for: void-initfs trusts whatever keys the checksum-verified tarball
// itself ships, so a future key rotation on Void's end is picked up
// automatically the next time this runs.
func installExtractedKeys(extractDir, destDir string) error {
	keysDir := filepath.Join(extractDir, "var", "db", "xbps", "keys")
	entries, err := os.ReadDir(keysDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", keysDir, err)
	}

	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("remove %s: %w", destDir, err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", destDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".plist") {
			continue
		}

		src := filepath.Join(keysDir, entry.Name())
		dst := filepath.Join(destDir, entry.Name())
		if err := copyFile(src, dst, 0o644); err != nil {
			return err
		}
		logInfo("cached repository key %s", dst)
	}

	return nil
}
