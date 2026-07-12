package main

import (
	"errors"
	"os"
)

// fatal logs err at ERROR level, closes the log, and exits the process
// with a non-zero status.
func fatal(err error) {
	logError("%v", err)
	closeLog()
	os.Exit(1)
}

func main() {
	defer closeLog()

	logInfo("starting")

	// Everything void-init does needs root (mounting datasources,
	// sethostname, chpasswd, writing under /etc), so fail with one clear
	// message up front when run by hand as a regular user.
	if os.Geteuid() != 0 {
		fatal(errors.New("void-init must run as root"))
	}

	userData, err := FindUserData()
	if err != nil {
		fatal(err)
	}

	config, err := ParseUserData(userData)
	if err != nil {
		fatal(err)
	}

	if err := ApplyUserData(config); err != nil {
		fatal(err)
	}

	var networkConfig *NetworkConfig
	if networkConfigData, err := FindNetworkConfig(); err == nil {
		networkConfig, err = ParseNetworkConfig(networkConfigData)
		if err != nil {
			fatal(err)
		}

		if err := ApplyNetworkConfig(networkConfig); err != nil {
			fatal(err)
		}
	} else {
		logInfo("no network-config found, skipping network setup: %v", err)
	}

	if err := ApplyHosts(config, staticAddress(networkConfig)); err != nil {
		fatal(err)
	}

	if err := enableService("qemu-ga"); err != nil {
		fatal(err)
	}

	logInfo("finished successfully")
}
