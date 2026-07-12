package main

import "fmt"

// repoAndArch returns the XBPS repository URL and XBPS_ARCH value for the
// given libc variant, per void-mkinitfs.md step 5.
func repoAndArch(libc string) (repo, arch string, err error) {
	switch libc {
	case "glibc":
		return "https://repo-default.voidlinux.org/current", "x86_64", nil
	case "musl":
		return "https://repo-default.voidlinux.org/current/musl", "x86_64-musl", nil
	default:
		return "", "", fmt.Errorf("unknown libc variant %q", libc)
	}
}

// packages is the proposed package set from void-mkinitfs.md step 5.
// Exact names should be verified against the live repo (xbps-query -R)
// before relying on this list - see the plan's "Open items" section.
func packages(l layout) []string {
	common := []string{
		"base-minimal", "linux", "dracut", "runit-void", "dhcpcd",
		"iproute2", "openssh", "shadow", "e2fsprogs", "dosfstools",
		"ca-certificates", "iana-etc",
	}

	if l == layoutEFI {
		return append(common, "grub-x86_64-efi")
	}
	return append(common, "grub")
}

// bootstrap installs packages into root via xbps-install, per
// void-mkinitfs.md step 5. There's no intermediate rootfs directory:
// xbps-install targets the mounted partition stack directly, since
// xbps-install -r with a foreign root just unpacks package files - it
// doesn't run pre/post install scriptlets, which is the gap reconfigure
// (step 6) fills afterward.
func bootstrap(root string, l layout, libc string) error {
	repo, arch, err := repoAndArch(libc)
	if err != nil {
		return err
	}

	logInfo("bootstrapping %s packages (%s, %s) into %s", libc, arch, repo, root)

	args := append([]string{
		"xbps-install", "-S",
		"-R", repo,
		"-r", root,
	}, packages(l)...)

	_, err = runCommandEnv([]string{"XBPS_ARCH=" + arch}, args...)
	return err
}
