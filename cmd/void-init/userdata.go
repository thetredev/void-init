package main

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

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

	if err := userData.validate(); err != nil {
		return nil, fmt.Errorf("invalid user-data: %w", err)
	}

	return &userData, nil
}

// validate rejects field values that would corrupt what they're later
// spliced into: hostname and fqdn end up on single lines in
// /etc/hostname and /etc/hosts, user and password become one
// "user:hash" chpasswd stdin record, and each SSH key becomes one
// authorized_keys line. YAML strings can legally contain newlines and
// other control characters, so without this a crafted value could
// inject extra lines or records; none of these values legitimately
// contain such characters (only SSH key comments may contain spaces).
func (u *UserData) validate() error {
	if strings.ContainsFunc(u.Hostname, isSpaceOrControl) {
		return fmt.Errorf("hostname %q contains whitespace or control characters", u.Hostname)
	}
	if strings.ContainsFunc(u.FQDN, isSpaceOrControl) {
		return fmt.Errorf("fqdn %q contains whitespace or control characters", u.FQDN)
	}
	if strings.ContainsFunc(u.User, isSpaceOrControl) ||
		strings.ContainsRune(u.User, ':') || strings.HasPrefix(u.User, "-") {
		return fmt.Errorf("user %q contains characters unsafe for a username", u.User)
	}
	// The password hash is deliberately not echoed into the error.
	if strings.ContainsFunc(u.Password, isSpaceOrControl) {
		return fmt.Errorf("password hash contains whitespace or control characters")
	}
	for i, key := range u.SSHAuthorizedKeys {
		if strings.ContainsFunc(key, unicode.IsControl) {
			return fmt.Errorf("ssh_authorized_keys entry %d contains control characters", i)
		}
	}

	return nil
}

// isSpaceOrControl reports whether r is any whitespace or control
// character - the classes that break single-line/single-token file
// formats and colon-separated records.
func isSpaceOrControl(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r)
}
