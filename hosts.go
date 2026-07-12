package main

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed templates/hosts
var hostsTemplate string

// hostsPath is the location of the system hosts file.
const hostsPath = "/etc/hosts"

// hostsTemplateData is the set of values substituted into the hosts
// template.
type hostsTemplateData struct {
	Address  string
	Hostname string
	FQDN     string
}

// ApplyHosts renders /etc/hosts from the hosts template, resolving the
// configured hostname/FQDN against address: the machine's static IP if one
// was configured, or the loopback alias 127.0.1.1 for dynamic addressing.
func ApplyHosts(u *UserData, address string) error {
	if !u.ManageEtcHosts || u.Hostname == "" {
		return nil
	}

	tmpl, err := template.New("hosts").Parse(hostsTemplate)
	if err != nil {
		return fmt.Errorf("parse hosts template: %w", err)
	}

	var rendered strings.Builder
	data := hostsTemplateData{
		Address:  address,
		Hostname: u.Hostname,
		FQDN:     u.FQDN,
	}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return fmt.Errorf("render hosts template: %w", err)
	}

	return writeManagedFile(hostsPath, rendered.String(), 0o644)
}

// staticAddress returns the first statically configured interface address
// in nc, or "127.0.1.1" if none is configured (i.e. addressing is dynamic).
func staticAddress(nc *NetworkConfig) string {
	if nc != nil {
		for _, device := range nc.Config {
			for _, subnet := range device.Subnets {
				if subnet.Type == "static" || subnet.Type == "static6" {
					address, _, _ := strings.Cut(subnet.Address, "/")
					return address
				}
			}
		}
	}

	return "127.0.1.1"
}
