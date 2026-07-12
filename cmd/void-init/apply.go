package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const hostnamePath = "/etc/hostname"

// ApplyUserData applies the parts of a parsed #cloud-config that mutate
// local system state: the hostname, the target user's password hash, and
// SSH authorized keys.
func ApplyUserData(u *UserData) error {
	if u.Hostname != "" {
		if err := applyHostname(u.Hostname); err != nil {
			return err
		}
	}

	if u.User == "" {
		return nil
	}

	if u.Password != "" {
		if err := applyPassword(u.User, u.Password); err != nil {
			return err
		}
	}

	if len(u.SSHAuthorizedKeys) > 0 {
		if err := applySSHAuthorizedKeys(u.User, u.SSHAuthorizedKeys); err != nil {
			return err
		}
	}

	return nil
}

// applyHostname writes /etc/hostname and applies it to the running kernel,
// mirroring `hostnamectl set-hostname <hostname>` on a minimal system.
func applyHostname(hostname string) error {
	logInfo("setting hostname to %q", hostname)

	if err := os.WriteFile(hostnamePath, []byte(withSingleTrailingNewline(hostname)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", hostnamePath, err)
	}

	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		return fmt.Errorf("set hostname: %w", err)
	}

	return nil
}

// applyPassword sets the given user's password hash by piping one
// "user:hash" record into `chpasswd -e` on stdin. usermod -p would be
// the more obvious tool, but it takes the hash as a command-line
// argument, and argv is world-readable via /proc/<pid>/cmdline for as
// long as the command runs - stdin leaks nothing. The record format is
// safe to build by concatenation because ParseUserData rejects
// usernames containing ':' and any whitespace/control characters in
// either field.
func applyPassword(username, hash string) error {
	logInfo("setting password for user %s", username)

	cmd := exec.Command("chpasswd", "-e")
	cmd.Stdin = strings.NewReader(username + ":" + hash + "\n")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chpasswd %s: %w: %s", username, err, output)
	}

	return nil
}

// applySSHAuthorizedKeys writes the given keys to the user's
// ~/.ssh/authorized_keys, creating the .ssh directory if needed and
// ensuring ownership/permissions match sshd's expectations.
func applySSHAuthorizedKeys(username string, keys []string) error {
	logInfo("installing %d SSH authorized key(s) for user %s", len(keys), username)

	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("lookup user %s: %w", username, err)
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("parse uid %s: %w", u.Uid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("parse gid %s: %w", u.Gid, err)
	}

	sshDir := filepath.Join(u.HomeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", sshDir, err)
	}
	if err := os.Chown(sshDir, uid, gid); err != nil {
		return fmt.Errorf("chown %s: %w", sshDir, err)
	}

	authorizedKeysPath := filepath.Join(sshDir, "authorized_keys")

	content := strings.Join(keys, "\n") + "\n\n" + userConfigMarker

	if err := writeManagedFile(authorizedKeysPath, content, 0o600); err != nil {
		return err
	}
	if err := os.Chown(authorizedKeysPath, uid, gid); err != nil {
		return fmt.Errorf("chown %s: %w", authorizedKeysPath, err)
	}

	return nil
}
