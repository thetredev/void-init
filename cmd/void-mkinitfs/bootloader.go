package main

import "fmt"

// installBootloader installs GRUB into root and generates its config, per
// void-mkinitfs.md step 9. grub-install/grub-probe need to see the real
// block device to write the boot sector / determine the device map,
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

	return nspawn(root, bind, "grub-mkconfig", "-o", "/boot/grub/grub.cfg")
}
