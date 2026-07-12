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

	fmt.Printf("%+v\n", config)
}

// TODO:
// - use Go template engine maybe if feasible for network configs etc
// - apply <user> pw: `usermod -p '<hash>' <user>`
// - add SSH keys
// - provide an option to generate a bootable, cloud-init ready voidlinux rootfs from scratch*

// * that rootfs shall have the current release of void-init installed as well, of course
