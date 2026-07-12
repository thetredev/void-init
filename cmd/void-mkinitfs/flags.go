package main

import (
	"flag"
	"fmt"
	"os"
)

// config holds void-mkinitfs's parsed and validated CLI flags, per
// void-mkinitfs.md's "CLI" section.
type config struct {
	bios                bool
	efi                 bool
	libc                string
	output              string
	image               string
	reinstallBootloader bool
	voidInitBinary      string
	updateXbps          bool
}

// parseFlags parses and validates os.Args.
func parseFlags() (*config, error) {
	cfg := &config{}

	flag.BoolVar(&cfg.bios, "bios", false, "target a legacy BIOS (GPT) image")
	flag.BoolVar(&cfg.efi, "efi", false, "target a UEFI image")
	flag.StringVar(&cfg.libc, "libc", "glibc", `libc variant to bootstrap: "glibc" or "musl"`)
	flag.StringVar(&cfg.output, "o", "", "output qcow2 path (build from scratch)")
	flag.StringVar(&cfg.output, "output", "", "same as -o")
	flag.StringVar(&cfg.image, "i", "", "reuse an existing qcow2 instead of building one")
	flag.StringVar(&cfg.image, "image", "", "same as -i")
	flag.BoolVar(&cfg.reinstallBootloader, "reinstall-bootloader", false, "with -i, also reinstall the bootloader (step 9)")
	flag.StringVar(&cfg.voidInitBinary, "void-init-binary", "void-init", "path to a built void-init binary to install into the image")
	flag.BoolVar(&cfg.updateXbps, "update-xbps", false, "force a re-download/re-verify of the cached xbps tools and repository keys from Void's static archive")

	flag.Parse()

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate enforces the flag combinations described in void-mkinitfs.md's
// "CLI" section: --bios/--efi are mutually exclusive and, when not using
// -i, exactly one is required; -i and -o are mutually exclusive;
// --reinstall-bootloader only makes sense alongside -i (the from-scratch
// path always installs the bootloader, per step 9).
func (c *config) validate() error {
	if c.bios && c.efi {
		return fmt.Errorf("--bios and --efi are mutually exclusive")
	}

	if c.image != "" {
		if c.output != "" {
			return fmt.Errorf("-i and -o are mutually exclusive")
		}
		if c.updateXbps {
			return fmt.Errorf("--update-xbps only applies when building from scratch (no packages are bootstrapped with -i)")
		}
		if _, err := os.Stat(c.image); err != nil {
			return fmt.Errorf("-i %s: %w", c.image, err)
		}
		return nil
	}

	if c.reinstallBootloader {
		return fmt.Errorf("--reinstall-bootloader only applies with -i")
	}
	if c.output == "" {
		return fmt.Errorf("-o is required (or -i to reuse an existing image)")
	}
	if _, err := os.Stat(c.output); err == nil {
		return fmt.Errorf("-o %s: already exists", c.output)
	}
	if !c.bios && !c.efi {
		return fmt.Errorf("exactly one of --bios or --efi is required")
	}
	if c.libc != "glibc" && c.libc != "musl" {
		return fmt.Errorf("--libc must be %q or %q, got %q", "glibc", "musl", c.libc)
	}

	return nil
}

// requestedLayout returns the layout explicitly requested via
// --bios/--efi, and whether one was given at all (false when neither flag
// was passed, which is only valid alongside -i, where the layout is
// inferred instead - see detectLayout).
func (c *config) requestedLayout() (l layout, ok bool) {
	switch {
	case c.bios:
		return layoutBIOS, true
	case c.efi:
		return layoutEFI, true
	default:
		return 0, false
	}
}
