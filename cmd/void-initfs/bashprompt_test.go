package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetBashPrompt(t *testing.T) {
	tests := []struct {
		name    string
		initial string // "" means the file doesn't exist yet
	}{
		{
			name:    "missing file gets the full template",
			initial: "",
		},
		{
			name: "unindented PS1 line gets replaced in place",
			initial: "# .bashrc\n" +
				"[ -z \"$PS1\" ] && return\n" +
				"alias ls='ls --color=auto'\n" +
				"PS1='[\\u@\\h \\W]\\$ '\n",
		},
		{
			name: "indented PS1 line gets replaced in place",
			initial: "# .bashrc\n" +
				"if [ -n \"$PS1\" ]; then\n" +
				"    PS1='[\\u@\\h \\W]\\$ '\n" +
				"fi\n",
		},
		{
			name:    "file with no PS1 line gets the full template",
			initial: "# .bashrc\nalias ls='ls --color=auto'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".bashrc")
			if tt.initial != "" {
				if err := os.WriteFile(path, []byte(tt.initial), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			if err := setBashPrompt(path); err != nil {
				t.Fatalf("setBashPrompt() error = %v", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(string(got), bashPromptLine) {
				t.Errorf("setBashPrompt() result = %q, want it to contain %q", got, bashPromptLine)
			}

			if strings.Count(string(got), "PS1=") != 1 {
				t.Errorf("setBashPrompt() result = %q, want exactly one PS1= line", got)
			}

			if tt.name == "unindented PS1 line gets replaced in place" ||
				tt.name == "indented PS1 line gets replaced in place" {
				if !strings.Contains(string(got), "alias ls='ls --color=auto'") &&
					!strings.Contains(string(got), "if [ -n \"$PS1\" ]; then") {
					t.Errorf("setBashPrompt() result = %q, want surrounding content preserved", got)
				}
			}
		})
	}
}

func TestConfigureBashPrompt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc", "skel"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "root"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := configureBashPrompt(root); err != nil {
		t.Fatalf("configureBashPrompt() error = %v", err)
	}

	skelProfile, err := os.ReadFile(filepath.Join(root, "etc", "skel", ".profile"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(skelProfile), "[ -f ~/.bashrc ] && . ~/.bashrc\n"; got != want {
		t.Errorf("/etc/skel/.profile = %q, want %q", got, want)
	}

	info, err := os.Stat(filepath.Join(root, "etc", "skel", ".profile"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("/etc/skel/.profile mode = %v, want 0644", info.Mode().Perm())
	}

	rootProfile, err := os.ReadFile(filepath.Join(root, "root", ".profile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rootProfile) != string(skelProfile) {
		t.Errorf("/root/.profile = %q, want it to match /etc/skel/.profile %q", rootProfile, skelProfile)
	}

	rootBashrc, err := os.ReadFile(filepath.Join(root, "root", ".bashrc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rootBashrc), bashPromptLine) {
		t.Errorf("/root/.bashrc = %q, want it to contain %q", rootBashrc, bashPromptLine)
	}
}
