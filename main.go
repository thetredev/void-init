package main

import (
	"fmt"
	"os"
)

func main() {
	userData, err := FindUserData()
	if err != nil {
		fmt.Fprintln(os.Stderr, "void-init:", err)
		os.Exit(1)
	}

	config, err := ParseUserData(userData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "void-init:", err)
		os.Exit(1)
	}

	if err := ApplyUserData(config); err != nil {
		fmt.Fprintln(os.Stderr, "void-init:", err)
		os.Exit(1)
	}

	var networkConfig *NetworkConfig
	if networkConfigData, err := FindNetworkConfig(); err == nil {
		networkConfig, err = ParseNetworkConfig(networkConfigData)
		if err != nil {
			fmt.Fprintln(os.Stderr, "void-init:", err)
			os.Exit(1)
		}

		if err := ApplyNetworkConfig(networkConfig); err != nil {
			fmt.Fprintln(os.Stderr, "void-init:", err)
			os.Exit(1)
		}
	}

	if err := ApplyHosts(config, staticAddress(networkConfig)); err != nil {
		fmt.Fprintln(os.Stderr, "void-init:", err)
		os.Exit(1)
	}
}

// TODO:
// 2) provide an option to generate a bootable, cloud-init ready voidlinux rootfs from scratch*

// * that rootfs shall have the current release of void-init installed as well, of course
