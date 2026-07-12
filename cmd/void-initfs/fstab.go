package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed templates/etc/fstab
var fstabTemplate string

// fstabTemplateData is the set of values substituted into the fstab
// template. EFIUUID is empty for the BIOS layout.
type fstabTemplateData struct {
	RootUUID string
	BootUUID string
	EFIUUID  string
}

// partitionUUID returns the UUID blkid reports for dev.
func partitionUUID(dev string) (string, error) {
	output, err := runCommand("blkid", "-s", "UUID", "-o", "value", dev)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// writeFstab renders /etc/fstab into root, listing every partition in l's
// layout by UUID.
func writeFstab(root string, l layout) error {
	rootUUID, err := partitionUUID(rootPartitionDevice(l))
	if err != nil {
		return err
	}
	bootUUID, err := partitionUUID(bootPartitionDevice(l))
	if err != nil {
		return err
	}

	data := fstabTemplateData{RootUUID: rootUUID, BootUUID: bootUUID}

	if l == layoutEFI {
		efiUUID, err := partitionUUID(efiPartitionDevice())
		if err != nil {
			return err
		}
		data.EFIUUID = efiUUID
	}

	tmpl, err := template.New("fstab").Parse(fstabTemplate)
	if err != nil {
		return fmt.Errorf("parse fstab template: %w", err)
	}

	var rendered strings.Builder
	if err := tmpl.Execute(&rendered, data); err != nil {
		return fmt.Errorf("render fstab template: %w", err)
	}

	path := root + "/etc/fstab"
	if err := writeFile(path, rendered.String(), 0o644); err != nil {
		return err
	}

	logInfo("wrote %s", path)
	return nil
}
