package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureShutdownAliases(t *testing.T) {
	root := t.TempDir()

	if err := configureShutdownAliases(root); err != nil {
		t.Fatalf("configureShutdownAliases() error = %v", err)
	}

	dir := filepath.Join(root, "etc", "bash", "bashrc.d")

	reboot, err := os.ReadFile(filepath.Join(dir, "reboot.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(reboot), "alias reboot='shutdown -r now && exit'\n"; got != want {
		t.Errorf("reboot.sh = %q, want %q", got, want)
	}

	poweroff, err := os.ReadFile(filepath.Join(dir, "poweroff.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(poweroff), "alias poweroff='shutdown -h now && exit'\n"; got != want {
		t.Errorf("poweroff.sh = %q, want %q", got, want)
	}
}
