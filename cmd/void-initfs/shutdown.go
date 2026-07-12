package main

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed templates/reboot.sh
var rebootScriptTemplate string

//go:embed templates/poweroff.sh
var poweroffScriptTemplate string

// configureShutdownAliases writes reboot.sh and poweroff.sh into
// /etc/bash/bashrc.d, aliasing reboot/poweroff to `shutdown -r/-h now &&
// exit`. Plain reboot(8)/poweroff(8) get killed along with the SSH
// session that invoked them before runit finishes shutting the machine
// down; routing through shutdown(8) and exiting the shell instead lets
// the session close cleanly while the shutdown proceeds, per TODO.md.
func configureShutdownAliases(root string) error {
	dir := filepath.Join(root, "etc", "bash", "bashrc.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	rebootScript := filepath.Join(dir, "reboot.sh")
	if err := writeFile(rebootScript, rebootScriptTemplate, 0o644); err != nil {
		return err
	}
	logInfo("wrote %s", rebootScript)

	poweroffScript := filepath.Join(dir, "poweroff.sh")
	if err := writeFile(poweroffScript, poweroffScriptTemplate, 0o644); err != nil {
		return err
	}
	logInfo("wrote %s", poweroffScript)

	return nil
}
