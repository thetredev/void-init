package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseUserDataSample(t *testing.T) {
	data, err := os.ReadFile("testfiles/user-data")
	if err != nil {
		t.Fatal(err)
	}

	userData, err := ParseUserData(data)
	if err != nil {
		t.Fatal(err)
	}

	if userData.Hostname != "template-vm" {
		t.Errorf("Hostname = %q, want %q", userData.Hostname, "template-vm")
	}
	if userData.User != "root" {
		t.Errorf("User = %q, want %q", userData.User, "root")
	}
	if len(userData.SSHAuthorizedKeys) != 1 {
		t.Errorf("len(SSHAuthorizedKeys) = %d, want 1", len(userData.SSHAuthorizedKeys))
	}
}

func TestParseUserDataRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "hostname with newline",
			yaml:    "hostname: \"a\\nb\"",
			wantErr: "hostname",
		},
		{
			name:    "fqdn with space",
			yaml:    "fqdn: \"a b.example.com\"",
			wantErr: "fqdn",
		},
		{
			name:    "username with colon",
			yaml:    "user: \"root:x\"",
			wantErr: "user",
		},
		{
			name:    "username with leading dash",
			yaml:    "user: \"-root\"",
			wantErr: "user",
		},
		{
			name:    "username with newline",
			yaml:    "user: \"root\\nevil\"",
			wantErr: "user",
		},
		{
			name:    "password hash with newline",
			yaml:    "password: \"$6$x\\nevil:hash\"",
			wantErr: "password",
		},
		{
			name:    "ssh key with newline",
			yaml:    "ssh_authorized_keys:\n  - \"ssh-ed25519 AAAA\\nssh-rsa BBBB\"",
			wantErr: "ssh_authorized_keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseUserData([]byte("#cloud-config\n" + tt.yaml + "\n"))
			if err == nil {
				t.Fatal("ParseUserData() succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ParseUserData() error = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseUserDataDoesNotEchoPasswordHash(t *testing.T) {
	_, err := ParseUserData([]byte("#cloud-config\npassword: \"secret\\nhash\"\n"))
	if err == nil {
		t.Fatal("ParseUserData() succeeded, want error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("ParseUserData() error %q echoes the password hash", err)
	}
}
