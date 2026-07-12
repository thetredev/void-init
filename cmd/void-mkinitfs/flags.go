package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// config holds void-mkinitfs's parsed and validated CLI flags.
type config struct {
	bios                bool
	efi                 bool
	libc                string
	output              string
	image               string
	reinstallBootloader bool
	voidInitBinary      string
	updateXbps          bool
	assumeYes           bool
	force               bool
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
	flag.BoolVar(&cfg.assumeYes, "y", false, "assume yes to the xbps tools/keys download confirmation")
	flag.BoolVar(&cfg.assumeYes, "yes", false, "same as -y")
	flag.BoolVar(&cfg.force, "f", false, "with -o, remove an existing file at the output path instead of failing")
	flag.BoolVar(&cfg.force, "force", false, "same as -f")

	flag.Parse()

	if flag.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument(s): %v (every flag's value must immediately follow it)", flag.Args())
	}
	if err := cfg.checkSwallowedValues(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// checkSwallowedValues guards against flag ordering silently changing
// behavior. The standard flag package's grammar has a footgun: a
// string flag like --void-init-binary always consumes the very next
// argument as its value, even if that argument was meant to be a
// separate flag - e.g. "--void-init-binary -y" sets voidInitBinary to
// the literal string "-y" instead of also setting assumeYes, with no
// error. None of this CLI's string-flag values are ever legitimately
// expected to start with "-", so one that does is almost certainly a
// flag that got swallowed as somebody else's value because of where it
// was placed on the command line - fail loudly instead of silently
// misbehaving differently depending on argument order.
func (c *config) checkSwallowedValues() error {
	suspects := []struct{ name, val string }{
		{"-o/--output", c.output},
		{"-i/--image", c.image},
		{"--libc", c.libc},
		{"--void-init-binary", c.voidInitBinary},
	}
	for _, s := range suspects {
		if strings.HasPrefix(s.val, "-") {
			return fmt.Errorf("%s got %q, which looks like a flag rather than a value - check that a value immediately follows %s", s.name, s.val, s.name)
		}
	}
	return nil
}

// validate enforces the CLI's flag combinations: --bios/--efi are
// mutually exclusive and, when not using -i, exactly one is required;
// -i and -o are mutually exclusive;
// --reinstall-bootloader/--update-xbps/-f/--force only make sense
// alongside -o (the from-scratch path). With -f, an existing -o target is
// allowed to pass validation, but isn't removed here - that happens later
// in createImage, since validate is meant to be side-effect-free.
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
		if c.force {
			return fmt.Errorf("-f/--force only applies to -o (there's no output file to overwrite with -i)")
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
	if _, err := os.Stat(c.output); err == nil && !c.force {
		return fmt.Errorf("-o %s: already exists (use -f/--force to overwrite)", c.output)
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
