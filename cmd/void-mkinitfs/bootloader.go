package main

import "fmt"

// installBootloader installs GRUB into root and generates its config, per
// void-mkinitfs.md step 9. grub-install needs to see the real block
// device to write the boot sector / determine the device map, which
// nspawn's private /dev doesn't include by default, hence the explicit
// bind mount.
func installBootloader(root string, l layout) error {
	bind := []string{"--bind=" + nbdDevice}

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

	return nspawn(root, nil, "grub-mkconfig", "-o", "/boot/grub/grub.cfg")
}
