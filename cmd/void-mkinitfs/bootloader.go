package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// installBootloader installs GRUB into root and generates its config.
// grub-install/grub-probe need to see the real block device to write
// the boot sector / determine the device map,
// which nspawn's private /dev doesn't include by default, hence the
// explicit bind mounts - of both nbdDevice itself and every partition
// node. grub-probe canonicalizes whatever device backs the mount it's
// asked about (/dev/nbd0pN, per /proc/self/mountinfo) and fails if that
// specific node isn't present, not just the parent disk - grub-install
// probes /boot (nbd0p2 on --bios), grub-mkconfig probes / too (nbd0p3),
// so both nspawn invocations need the same full bind list, not just the
// first.
func installBootloader(root string, l layout) error {
	bind := []string{"--bind=" + nbdDevice}
	for n := 1; n <= l.partitionCount(); n++ {
		bind = append(bind, "--bind="+partitionDevice(n))
	}

	var grubArgs []string
	switch l {
	case layoutBIOS:
		grubArgs = []string{"grub-install", "--target=i386-pc", "--boot-directory=/boot", nbdDevice}
	case layoutEFI:
		grubArgs = []string{
			"grub-install", "--target=x86_64-efi",
			"--efi-directory=/boot/efi", "--boot-directory=/boot", "--removable",
		}
	default:
		return fmt.Errorf("unknown layout %v", l)
	}

	logInfo("installing bootloader (%s) into %s", l, root)
	if err := nspawn(root, bind, grubArgs...); err != nil {
		return err
	}

	mkconfig, err := grubMkconfigCommand(l)
	if err != nil {
		return err
	}
	return nspawn(root, bind, mkconfig...)
}

// grubMkconfigCommand builds the shell command grub-mkconfig runs under,
// prefixed with commands that recreate udev's /dev/disk/by-uuid symlinks
// for the boot and root partitions. grub-mkconfig's 10_linux script only
// emits "root=UUID=..." (and the matching search command for /boot) if
// that symlink exists; otherwise it silently falls back to the raw device
// path it resolved at generation time. systemd-nspawn's private /dev never
// gets those symlinks on its own - nothing runs udev inside it for the
// bound partition nodes - so without this, the generated grub.cfg
// hardcodes something like root=/dev/nbd0p3: a device name that only
// exists on the build host and is meaningless inside the booted VM,
// leaving dracut unable to find the root filesystem at boot.
func grubMkconfigCommand(l layout) ([]string, error) {
	commands := []string{"mkdir -p /dev/disk/by-uuid"}

	for _, dev := range []string{bootPartitionDevice(l), rootPartitionDevice(l)} {
		uuid, err := partitionUUID(dev)
		if err != nil {
			return nil, err
		}
		commands = append(commands, byUUIDSymlink(dev, uuid))
	}

	commands = append(commands, "grub-mkconfig -o /boot/grub/grub.cfg")
	return []string{"sh", "-c", strings.Join(commands, " && ")}, nil
}

// byUUIDSymlink formats the shell command to recreate udev's
// /dev/disk/by-uuid symlink for a partition device node.
func byUUIDSymlink(dev, uuid string) string {
	return fmt.Sprintf("ln -sf ../../%s /dev/disk/by-uuid/%s", filepath.Base(dev), uuid)
}
