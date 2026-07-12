package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
)

// ApplyUserData applies the parts of a parsed #cloud-config that mutate
// local system state: the target user's password hash and SSH authorized
// keys.
func ApplyUserData(u *UserData) error {
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

// applyPassword sets the given user's password hash via usermod, mirroring
// `usermod -p '<hash>' <user>`.
func applyPassword(username, hash string) error {
	cmd := exec.Command("usermod", "-p", hash, username)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("usermod %s: %w: %s", username, err, output)
	}

	return nil
}

// applySSHAuthorizedKeys writes the given keys to the user's
// ~/.ssh/authorized_keys, creating the .ssh directory if needed and
// ensuring ownership/permissions match sshd's expectations.
func applySSHAuthorizedKeys(username string, keys []string) error {
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

	content := ""
	for _, key := range keys {
		content += key + "\n"
	}

	if err := os.WriteFile(authorizedKeysPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", authorizedKeysPath, err)
	}
	if err := os.Chown(authorizedKeysPath, uid, gid); err != nil {
		return fmt.Errorf("chown %s: %w", authorizedKeysPath, err)
	}

	return nil
}
