package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// installRepoKeys pre-seeds root's XBPS key store with Void's repository
// signing keys, per void-mkinitfs.md step 5. xbps-install's key trust is
// scoped per-rootdir rather than host-global, and root here is always a
// freshly created rootdir that has never trusted anything - without this,
// xbps-install stops on an interactive "import this public key?" prompt
// it can't read an answer to (void-mkinitfs isn't attached to a TTY),
// even with -y. Real Void hosts don't hit this because their host-wide
// /var/db/xbps/keys/ was seeded once, long ago; void-installer sidesteps
// the same problem by copying the host's trusted keys into its target
// root before bootstrapping it - this mirrors that, using localKeysDir
// (populated from Void's live static archive by ensureXbps, see xbps.go)
// as the "host" side. Only called for from-scratch builds - reusing an
// existing image (-i) never bootstraps packages, so it never needs keys.
func installRepoKeys(root string) error {
	entries, err := os.ReadDir(localKeysDir)
	if err != nil {
		return fmt.Errorf("read %s (expected ensureXbps to have populated it): %w", localKeysDir, err)
	}

	dir := filepath.Join(root, "var", "db", "xbps", "keys")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(localKeysDir, entry.Name())
		dst := filepath.Join(dir, entry.Name())
		if err := copyFile(src, dst, 0o644); err != nil {
			return err
		}
	}

	return nil
}
