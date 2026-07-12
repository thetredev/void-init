package main

// nspawn runs command inside root via systemd-nspawn, without --boot (a
// single command with an auto-provisioned private /proc, /sys, /dev, and
// /run - not a full container boot). extraArgs are inserted before the
// "--" separator (e.g. --bind=/dev/nbd0 or --resolv-conf=bind-host), for
// callers that need the nspawn environment to see something beyond the
// target root itself.
func nspawn(root string, extraArgs []string, command ...string) error {
	args := []string{"systemd-nspawn", "-D", root}
	args = append(args, extraArgs...)
	args = append(args, "--")
	args = append(args, command...)

	_, err := runCommand(args...)
	return err
}

// reconfigure runs xbps-reconfigure -fa inside root to execute every
// package trigger XBPS deferred during bootstrap (step 5) - initramfs
// generation via dracut, shadow's user/group setup, locale generation,
// ca-certificates bundling, etc. - since the install target wasn't the
// host's actual "/". Per void-mkinitfs.md step 6.
func reconfigure(root string) error {
	logInfo("reconfiguring packages in %s", root)
	return nspawn(root, []string{"--resolv-conf=bind-host"}, "xbps-reconfigure", "-fa")
}
