package main

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// cloudConfigHeader is the magic first line that marks a file as cloud-init
// user-data in the #cloud-config format, as opposed to a script (#!) or
// other cloud-init format.
const cloudConfigHeader = "#cloud-config"

// UserData covers the subset of #cloud-config keys Proxmox exposes on a
// VM's Cloud-Init page (see testfiles/user-data).
type UserData struct {
	Hostname          string   `yaml:"hostname"`
	FQDN              string   `yaml:"fqdn"`
	ManageEtcHosts    bool     `yaml:"manage_etc_hosts"`
	User              string   `yaml:"user"`
	DisableRoot       bool     `yaml:"disable_root"`
	Password          string   `yaml:"password"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys"`
	Chpasswd          struct {
		Expire bool `yaml:"expire"`
	} `yaml:"chpasswd"`
}

// ParseUserData parses raw #cloud-config user-data content.
func ParseUserData(data []byte) (*UserData, error) {
	firstLine, _, _ := bytes.Cut(data, []byte("\n"))
	if strings.TrimSpace(string(firstLine)) != cloudConfigHeader {
		return nil, fmt.Errorf("unsupported user-data format: missing %q header", cloudConfigHeader)
	}

	var userData UserData
	if err := yaml.Unmarshal(data, &userData); err != nil {
		return nil, fmt.Errorf("parse user-data: %w", err)
	}

	return &userData, nil
}
