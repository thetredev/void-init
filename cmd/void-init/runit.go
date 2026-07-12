package main

import (
	"fmt"
	"os"
)

// svDir and runsvdirCurrent describe the runit layout void uses to
// enable/disable services: a service is enabled by symlinking its
// definition in /etc/sv into the active runsvdir.
const (
	svDir           = "/etc/sv"
	runsvdirCurrent = "/etc/runit/runsvdir/current"
)

// enableService symlinks a service's /etc/sv definition into the active
// runsvdir, mirroring `ln -s /etc/sv/<name> /etc/runit/runsvdir/current/`.
func enableService(name string) error {
	link := runsvdirCurrent + "/" + name
	if _, err := os.Lstat(link); err == nil {
		logInfo("service %s already enabled", name)
		return nil
	}

	logInfo("enabling service %s", name)

	if err := os.Symlink(svDir+"/"+name, link); err != nil {
		return fmt.Errorf("enable service %s: %w", name, err)
	}

	return nil
}

// disableService removes a service's symlink from the active runsvdir,
// mirroring `rm /etc/runit/runsvdir/current/<name>`.
func disableService(name string) error {
	link := runsvdirCurrent + "/" + name
	if err := os.Remove(link); err != nil {
		if os.IsNotExist(err) {
			logInfo("service %s already disabled", name)
			return nil
		}
		return fmt.Errorf("disable service %s: %w", name, err)
	}

	logInfo("disabling service %s", name)
	return nil
}
