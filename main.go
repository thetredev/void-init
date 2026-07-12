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
}

// TODO:
// - use Go template engine maybe if feasible for network configs etc
// - provide an option to generate a bootable, cloud-init ready voidlinux rootfs from scratch*

// * that rootfs shall have the current release of void-init installed as well, of course
