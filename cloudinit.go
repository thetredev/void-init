package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// cloudInitSources are the device globs scraped for a cloud-init NoCloud
// datasource, in order. Proxmox attaches the generated ISO as a CD-ROM,
// which shows up as /dev/srX.
var cloudInitSources = []string{"/dev/sr*"}

// FindUserData scrapes the configured cloud-init sources for a user-data
// file and returns its raw contents. It mirrors stuff/void-init.sh: mount
// each candidate device read-only, check for /user-data, and stop at the
// first match.
func FindUserData() ([]byte, error) {
	return findCloudInitFile("user-data")
}

// FindNetworkConfig scrapes the configured cloud-init sources for a
// network-config file and returns its raw contents.
func FindNetworkConfig() ([]byte, error) {
	return findCloudInitFile("network-config")
}

// findCloudInitFile scrapes the configured cloud-init sources for the
// given filename and returns its raw contents. It mounts each candidate
// device read-only and stops at the first match.
func findCloudInitFile(name string) ([]byte, error) {
	var devices []string
	for _, pattern := range cloudInitSources {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		devices = append(devices, matches...)
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no cloud-init source devices found")
	}

	logInfo("scanning %v for %s", devices, name)

	mountpoint, err := os.MkdirTemp("", "void-init-cloud-init")
	if err != nil {
		return nil, fmt.Errorf("create mountpoint: %w", err)
	}
	defer os.RemoveAll(mountpoint)

	for _, device := range devices {
		data, err := readFileFromDevice(device, mountpoint, name)
		if err != nil {
			logWarn("%s: %v", device, err)
			continue
		}
		if data != nil {
			logInfo("found %s on %s", name, device)
			return data, nil
		}
	}

	return nil, fmt.Errorf("no cloud-init %s found on %v", name, devices)
}

// readFileFromDevice mounts device at mountpoint and reads the named file
// from it, if present. It returns (nil, nil) if the device mounts fine but
// carries no such file.
func readFileFromDevice(device, mountpoint, name string) ([]byte, error) {
	if err := syscall.Mount(device, mountpoint, "iso9660", syscall.MS_RDONLY, ""); err != nil {
		return nil, fmt.Errorf("mount %s: %w", device, err)
	}
	defer syscall.Unmount(mountpoint, 0)

	path := filepath.Join(mountpoint, name)
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}

	return os.ReadFile(path)
}
