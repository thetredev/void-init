package main

import (
	"fmt"
	"strings"
)

// repoAndArch returns the XBPS repository URL and XBPS_ARCH value for the
// given libc variant.
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

// packages is the package set installed into every image.
func packages(l layout, userPackages ...string) []string {
	packageList := []string{
		"base-system", "linux", "dracut", "runit-void", "dhcpcd",
		"iproute2", "openssh", "shadow", "e2fsprogs", "dosfstools",
		"ca-certificates", "iana-etc", "bash-completion", "net-tools",
		"qemu-ga", "binutils", "zstd",
	}

	packageList = append(packageList, userPackages...)

	if l == layoutEFI {
		return append(packageList, "grub-x86_64-efi")
	}
	return append(packageList, "grub")
}

// bootstrap installs packages into root via xbps-install. There's no
// intermediate rootfs directory: xbps-install targets the mounted
// partition stack directly, since xbps-install -r with a foreign root
// just unpacks package files - it doesn't run pre/post install
// scriptlets, which is the gap reconfigure (see nspawn.go) fills
// afterward. -y is required since void-initfs isn't
// attached to a TTY: on the first fetch against a repo whose signing key
// isn't already trusted on the host, xbps-install otherwise blocks on an
// interactive "import this public key?" prompt it can't read an answer
// to.
func bootstrap(root string, l layout, libc string, userPackages string) error {
	repo, arch, err := repoAndArch(libc)
	if err != nil {
		return err
	}

	logInfo("bootstrapping %s and user packages (%s, %s) into %s", libc, arch, repo, root)

	var _userPackages []string
	if userPackages != "" {
		_userPackages = strings.Split(userPackages, ",")
	}

	args := append([]string{
		"xbps-install", "-S", "-y",
		"-R", repo,
		"-r", root,
	}, packages(l, _userPackages...)...)

	_, err = runCommandEnv([]string{"XBPS_ARCH=" + arch}, args...)
	return err
}
