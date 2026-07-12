package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetFlags gives the test a clean flag.CommandLine (parseFlags
// registers flags on the package-level flag.CommandLine via
// flag.BoolVar/flag.StringVar, so reusing it across calls panics with
// "flag redefined") and sets os.Args, which flag.Parse reads implicitly.
// None of this touches the filesystem beyond what the test itself passes
// in as flag values, so it needs no root privileges.
func resetFlags(t *testing.T, args ...string) {
	t.Helper()

	fs := flag.NewFlagSet("void-mkinitfs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	flag.CommandLine = fs

	os.Args = append([]string{"void-mkinitfs"}, args...)
}

func TestParseFlagsErrors(t *testing.T) {
	existingImage := filepath.Join(t.TempDir(), "existing.qcow2")
	if err := os.WriteFile(existingImage, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missingImage := filepath.Join(t.TempDir(), "missing.qcow2")

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "bios and efi mutually exclusive",
			args:    []string{"--bios", "--efi", "-o", "out.qcow2"},
			wantErr: "--bios and --efi are mutually exclusive",
		},
		{
			name:    "i and o mutually exclusive",
			args:    []string{"-i", existingImage, "-o", "out.qcow2"},
			wantErr: "-i and -o are mutually exclusive",
		},
		{
			name:    "update-xbps requires building from scratch",
			args:    []string{"-i", existingImage, "--update-xbps"},
			wantErr: "--update-xbps only applies when building from scratch",
		},
		{
			name:    "force requires building from scratch",
			args:    []string{"-i", existingImage, "-f"},
			wantErr: "-f/--force only applies to -o",
		},
		{
			name:    "i with nonexistent image",
			args:    []string{"-i", missingImage},
			wantErr: "-i " + missingImage,
		},
		{
			name:    "reinstall-bootloader requires i",
			args:    []string{"--bios", "-o", "out.qcow2", "--reinstall-bootloader"},
			wantErr: "--reinstall-bootloader only applies with -i",
		},
		{
			name:    "o required without i",
			args:    []string{"--bios"},
			wantErr: "-o is required",
		},
		{
			name:    "o already exists without force",
			args:    []string{"--bios", "-o", existingImage},
			wantErr: "already exists",
		},
		{
			name:    "bios or efi required with o",
			args:    []string{"-o", "out.qcow2"},
			wantErr: "exactly one of --bios or --efi is required",
		},
		{
			name:    "invalid libc",
			args:    []string{"--bios", "-o", "out.qcow2", "--libc", "invalid"},
			wantErr: `--libc must be "glibc" or "musl", got "invalid"`,
		},
		{
			name:    "unexpected positional argument",
			args:    []string{"--bios", "-o", "out.qcow2", "extra"},
			wantErr: "unexpected argument(s)",
		},
		{
			name:    "flag value swallowed by preceding string flag",
			args:    []string{"--bios", "-o", "out.qcow2", "--void-init-binary", "-y"},
			wantErr: "looks like a flag rather than a value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags(t, tt.args...)

			_, err := parseFlags()
			if err == nil {
				t.Fatalf("parseFlags(%v) succeeded, want error containing %q", tt.args, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseFlags(%v) error = %q, want it to contain %q", tt.args, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseFlagsSuccess(t *testing.T) {
	existingImage := filepath.Join(t.TempDir(), "existing.qcow2")
	if err := os.WriteFile(existingImage, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, cfg *config)
	}{
		{
			name: "bios build from scratch",
			args: []string{"--bios", "--libc", "musl", "-o", filepath.Join(t.TempDir(), "out.qcow2")},
			check: func(t *testing.T, cfg *config) {
				if !cfg.bios || cfg.efi {
					t.Errorf("bios=%v efi=%v, want bios only", cfg.bios, cfg.efi)
				}
				if cfg.libc != "musl" {
					t.Errorf("libc = %q, want musl", cfg.libc)
				}
			},
		},
		{
			name: "efi build defaults to glibc",
			args: []string{"--efi", "-o", filepath.Join(t.TempDir(), "out.qcow2")},
			check: func(t *testing.T, cfg *config) {
				if !cfg.efi {
					t.Errorf("efi = false, want true")
				}
				if cfg.libc != "glibc" {
					t.Errorf("libc = %q, want default glibc", cfg.libc)
				}
			},
		},
		{
			name: "reuse existing image",
			args: []string{"-i", existingImage},
			check: func(t *testing.T, cfg *config) {
				if cfg.image != existingImage {
					t.Errorf("image = %q, want %q", cfg.image, existingImage)
				}
			},
		},
		{
			name: "force allows overwriting an existing output file",
			args: []string{"--bios", "-o", existingImage, "-f"},
			check: func(t *testing.T, cfg *config) {
				if !cfg.force {
					t.Errorf("force = false, want true")
				}
			},
		},
		{
			name: "output alias same as -o",
			args: []string{"--bios", "--output", filepath.Join(t.TempDir(), "out.qcow2")},
			check: func(t *testing.T, cfg *config) {
				if cfg.output == "" {
					t.Errorf("output not set via --output alias")
				}
			},
		},
		{
			name: "image alias same as -i",
			args: []string{"--image", existingImage},
			check: func(t *testing.T, cfg *config) {
				if cfg.image != existingImage {
					t.Errorf("image = %q, want %q", cfg.image, existingImage)
				}
			},
		},
		{
			name: "yes alias same as -y",
			args: []string{"--bios", "-o", filepath.Join(t.TempDir(), "out.qcow2"), "--yes"},
			check: func(t *testing.T, cfg *config) {
				if !cfg.assumeYes {
					t.Errorf("assumeYes = false, want true via --yes alias")
				}
			},
		},
		{
			name: "force alias same as -f",
			args: []string{"--bios", "-o", existingImage, "--force"},
			check: func(t *testing.T, cfg *config) {
				if !cfg.force {
					t.Errorf("force = false, want true via --force alias")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags(t, tt.args...)

			cfg, err := parseFlags()
			if err != nil {
				t.Fatalf("parseFlags(%v) = %v, want success", tt.args, err)
			}
			tt.check(t, cfg)
		})
	}
}

// TestParseFlagsOrderIndependent is a regression test: flag values used
// to silently differ depending on where -y/--update-xbps/etc. landed on
// the command line relative to other flags (see checkSwallowedValues).
// As long as every flag's value immediately follows it, the resulting
// config must be identical no matter what order the flags themselves are
// given in.
func TestParseFlagsOrderIndependent(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.qcow2")
	binary := filepath.Join(t.TempDir(), "void-init")

	orderings := [][]string{
		{"--bios", "-o", out, "-y", "--void-init-binary", binary, "--update-xbps"},
		{"-y", "--update-xbps", "--void-init-binary", binary, "--bios", "-o", out},
		{"--void-init-binary", binary, "-o", out, "--update-xbps", "-y", "--bios"},
		{"--update-xbps", "--bios", "-y", "-o", out, "--void-init-binary", binary},
	}

	var want *config
	for i, args := range orderings {
		resetFlags(t, args...)

		cfg, err := parseFlags()
		if err != nil {
			t.Fatalf("ordering %d (%v): parseFlags() = %v, want success", i, args, err)
		}

		if want == nil {
			want = cfg
			continue
		}

		if *cfg != *want {
			t.Fatalf("ordering %d (%v) produced %+v, want %+v (from ordering 0)", i, args, *cfg, *want)
		}
	}
}

func TestRequestedLayout(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config
		wantL  layout
		wantOK bool
	}{
		{"bios", config{bios: true}, layoutBIOS, true},
		{"efi", config{efi: true}, layoutEFI, true},
		{"neither", config{}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, ok := tt.cfg.requestedLayout()
			if l != tt.wantL || ok != tt.wantOK {
				t.Errorf("requestedLayout() = (%v, %v), want (%v, %v)", l, ok, tt.wantL, tt.wantOK)
			}
		})
	}
}
