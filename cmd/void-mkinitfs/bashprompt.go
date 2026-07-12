package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

//go:embed templates/profile
var profileTemplate string

//go:embed templates/bashrc
var bashrcTemplate string

// bashPromptLine is the PS1 assignment void-init expects in /root/.bashrc,
// per TODO.md's bash-prompt fix.
const bashPromptLine = `PS1='\[\e]0;\u@\h: \w\a\]\u@\h:\w\$ '`

// ps1LineRE matches an existing PS1= assignment line, regardless of
// indentation, so setBashPrompt can replace it in place.
var ps1LineRE = regexp.MustCompile(`(?m)^[ \t]*PS1=.*$`)

// configureBashPrompt writes /etc/skel/.profile (so new users' login
// shells source ~/.bashrc), copies it to /root/.profile, and makes sure
// /root/.bashrc sets bashPromptLine - per TODO.md's bash-prompt fix.
func configureBashPrompt(root string) error {
	skelProfile := filepath.Join(root, "etc", "skel", ".profile")
	if err := writeFile(skelProfile, profileTemplate, 0o644); err != nil {
		return err
	}
	logInfo("wrote %s", skelProfile)

	rootProfile := filepath.Join(root, "root", ".profile")
	if err := copyFile(skelProfile, rootProfile, 0o644); err != nil {
		return err
	}
	logInfo("wrote %s", rootProfile)

	rootBashrc := filepath.Join(root, "root", ".bashrc")
	if err := setBashPrompt(rootBashrc); err != nil {
		return err
	}
	logInfo("wrote %s", rootBashrc)

	return nil
}

// setBashPrompt replaces an existing PS1= line in path with
// bashPromptLine. If path doesn't exist yet, or exists but has no PS1=
// line to replace, it writes bashrcTemplate wholesale instead.
func setBashPrompt(path string) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeFile(path, bashrcTemplate, 0o644)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	if !ps1LineRE.Match(existing) {
		return writeFile(path, bashrcTemplate, 0o644)
	}

	replaced := ps1LineRE.ReplaceAll(existing, []byte(bashPromptLine))
	return writeFile(path, string(replaced), 0o644)
}
