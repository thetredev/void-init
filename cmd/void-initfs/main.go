package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	defer logger.Close()

	cfg, err := parseFlags()
	if err != nil {
		fatal(err)
	}

	if err := run(cfg); err != nil {
		fatal(err)
	}

	logInfo("finished successfully")
}

// fatal logs err at ERROR level and exits the process with a non-zero
// status.
func fatal(err error) {
	logError("%v", err)
	os.Exit(1)
}

// run executes the build-or-reuse pipeline: either building a fresh
// image from scratch (cfg.output) or reusing an existing one
// (cfg.image).
func run(cfg *config) error {
	stack := &cleanupStack{}
	defer stack.unwind()

	// A Ctrl-C mid-run still needs to detach the nbd device and unmount
	// partitions instead of leaking that state on the host, since none of
	// it is namespaced the way systemd-nspawn's own mounts are.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	go func() {
		if _, ok := <-sig; ok {
			logWarn("interrupted, cleaning up")
			stack.unwind()
			os.Exit(1)
		}
	}()

	if err := preflight(cfg.image == "", cfg.updateXbps, cfg.assumeYes); err != nil {
		return err
	}

	if cfg.image != "" {
		return runReuse(cfg, stack)
	}
	return runBuild(cfg, stack)
}

// runReuse attaches an existing image, infers its layout from partition
// count, mounts it, and refreshes void-init (and, if requested, the
// bootloader) without rebuilding the rootfs.
func runReuse(cfg *config, stack *cleanupStack) error {
	logInfo("reusing existing image %s", cfg.image)

	if err := attachImage(cfg.image); err != nil {
		return err
	}
	stack.push(detachImage)

	detected, err := detectLayout()
	if err != nil {
		return err
	}
	if requested, ok := cfg.requestedLayout(); ok && requested != detected {
		return fmt.Errorf("--%s given but partition count implies %s layout", requested, detected)
	}

	target, err := mount(detected, stack)
	if err != nil {
		return err
	}

	if err := installBinaries(target.root, cfg.binaryPath); err != nil {
		return err
	}

	if err := enableSSHService(target.root); err != nil {
		return err
	}

	if cfg.reinstallBootloader {
		if err := installBootloader(target.root, detected); err != nil {
			return err
		}
	}

	return nil
}

// runBuild builds a bootable, cloud-init-ready Void Linux qcow2 image
// from scratch.
func runBuild(cfg *config, stack *cleanupStack) error {
	l, _ := cfg.requestedLayout()

	logInfo("building a new %s image at %s (%s)", l, cfg.output, cfg.libc)

	if err := createImage(cfg.output, cfg.force); err != nil {
		return err
	}

	if err := attachImage(cfg.output); err != nil {
		return err
	}
	stack.push(detachImage)

	if err := partition(l); err != nil {
		return err
	}

	if err := makeFilesystems(l); err != nil {
		return err
	}

	target, err := mount(l, stack)
	if err != nil {
		return err
	}

	if err := installRepoKeys(target.root); err != nil {
		return err
	}

	if err := bootstrap(target.root, l, cfg.libc, cfg.packages); err != nil {
		return err
	}

	if err := reconfigure(target.root); err != nil {
		return err
	}

	if err := configureBashPrompt(target.root); err != nil {
		return err
	}

	if err := configureShutdownAliases(target.root); err != nil {
		return err
	}

	if err := writeFstab(target.root, l); err != nil {
		return err
	}

	if err := installBinaries(target.root, cfg.binaryPath); err != nil {
		return err
	}

	if err := enableSSHService(target.root); err != nil {
		return err
	}

	return installBootloader(target.root, l)
}
