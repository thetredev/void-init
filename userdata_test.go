package main

import (
	"os"
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
