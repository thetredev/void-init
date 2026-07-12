package main

import (
	"fmt"
	"os"
	"os/exec"
)

// requiredTools are the external tools the pipeline shells out to,
// checked up front so void-initfs fails fast with one clear error
// instead of dying halfway through a run. xbps-install/xbps-reconfigure
// are handled separately (see ensureXbps) since they have a download
// fallback and are only needed when building from scratch.
var requiredTools = []string{
	"systemd-nspawn",
	"qemu-img",
	"qemu-nbd",
	"sgdisk",
	"partprobe",
	"udevadm",
	"mkfs.vfat",
	"mkfs.ext2",
	"mkfs.ext4",
	"blkid",
	"mount",
	"umount",
	"grub-install",
	"grub-mkconfig",
}

// preflight checks that every external tool the pipeline needs is
// available before any work starts. If needXbps is set (building from
// scratch, as opposed to reusing an image via -i), it also ensures
// xbps-install/xbps-reconfigure and Void's repository signing keys are
// available, offering to download/verify them from Void's live static
// archive into /usr/local/bin and /usr/local/share/void-initfs/keys if
// missing - that check runs last. updateXbps (--update-xbps) forces
// that refresh even if both are
// already present; assumeYes (-y/--yes) skips its download confirmation.
func preflight(needXbps, updateXbps, assumeYes bool) error {
	// Everything downstream needs root (qemu-nbd, sgdisk, mkfs, mount,
	// systemd-nspawn), so fail with one clear message up front instead of
	// with whichever tool happens to hit EPERM first, partway through.
	if os.Geteuid() != 0 {
		return fmt.Errorf("void-initfs must run as root")
	}

	var missing []string
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tools: %v", missing)
	}

	if needXbps {
		if err := ensureXbps(updateXbps, assumeYes); err != nil {
			return err
		}
	}

	return nil
}
