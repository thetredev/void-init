package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// rcLocal is the /etc/rc.local void-initfs writes into every image.
// void-init does the rest of the work itself at first boot - there's no
// mirroring of that boot-time logic here.
const rcLocal = "#!/bin/sh\n/usr/local/bin/void-init\n"

// installBinaries copies the void-init and void-initfs binaries at binaryPath
// into root's /usr/local/bin and wires up /etc/rc.local to run it. sshd (already
// installed) generates its own host keys on first start via its own
// runit service - void-initfs does nothing SSH-key-related.
func installBinaries(root, binaryPath string) error {
	dst := filepath.Join(root, "usr", "local", "bin")

	logInfo("installing void-init and void-initfs binaries to %s", dst)

	binaries := []string{"void-init", "void-initfs"}
	for _, binary := range binaries {
		source := fmt.Sprintf("%s/%s", binaryPath, binary)
		destination := fmt.Sprintf("%s/%s", dst, binary)

		if err := copyFile(source, destination, 0o755); err != nil {
			return err
		}
	}

	rcLocalPath := filepath.Join(root, "etc", "rc.local")
	logInfo("writing %s", rcLocalPath)
	if err := writeFile(rcLocalPath, rcLocal, 0o755); err != nil {
		return err
	}

	return nil
}

// enableSSHService symlinks sshd into root's default runsvdir. dhcpcd is
// deliberately left disabled: void-init itself decides DHCP vs. static
// (and enables/disables dhcpcd accordingly) at first real boot, per
// network.go's ApplyNetworkConfig in cmd/void-init.
func enableSSHService(root string) error {
	link := filepath.Join(root, "etc", "runit", "runsvdir", "default", "sshd")
	target := "/etc/sv/sshd"

	if _, err := os.Lstat(link); err == nil {
		logInfo("sshd already enabled in %s", root)
		return nil
	}

	logInfo("enabling sshd in %s", root)
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("enable sshd: %w", err)
	}

	return nil
}
