package main

import (
	"fmt"
	"os/exec"
)

// requiredTools are the external tools the pipeline shells out to,
// checked up front so void-mkinitfs fails fast with one clear error
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
// xbps-install/xbps-reconfigure are available, offering to download
// static builds into /usr/local/bin if they're missing anywhere on PATH -
// that check runs last, per void-mkinitfs.md's CLI section.
func preflight(needXbps bool) error {
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
		if err := ensureXbps(); err != nil {
			return err
		}
	}

	return nil
}
