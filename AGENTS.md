# AGENTS.md

General knowledge and conventions for any LLM/agent working in this repository. Claude-specific
process notes live separately in `CLAUDE.md` — read both, this file first.

## What this project is

`void-init` is a small, single-purpose Go binary that acts as a minimal cloud-init substitute for
Void Linux. It's meant to run once per boot, very early, invoked directly from `/etc/rc.local`
(which `runit` executes before any services start — there is no supervising service, no syslog
daemon guaranteed present, nothing but the raw boot environment). It scrapes a Proxmox-generated
NoCloud cloud-init ISO (`user-data` + optional `network-config`) off `/dev/sr*`, and applies it:
hostname, user password hash, SSH authorized keys, network interface configuration (DHCP/SLAAC via
`dhcpcd`, static via `ip` directly), `/etc/resolv.conf`, and `/etc/hosts`.

There is a second binary, `void-mkinitfs`, which builds a bootable, cloud-init-ready Void Linux
qcow2 disk image from scratch (with `void-init` pre-baked in and `/etc/rc.local` wired up), so a VM
can be provisioned from that image and self-configure on first boot via `void-init`. It is fully
implemented (both BIOS and EFI layouts build and boot). Its design lives in the doc comments under
`cmd/void-mkinitfs/` — there is no separate planning document; a `void-mkinitfs.md` design doc
existed during initial development but was deliberately deleted once the implementation caught up
with it (see git history for `void-mkinitfs.md` if you need the original design rationale).

## Repo layout

A `cmd/` layout with two binaries sharing `internal/vlog` as their only common code:

**`cmd/void-init/`** — the boot-time binary:

| File | Responsibility |
|---|---|
| `main.go` | Entry point: find → parse → apply `user-data`, then `network-config`, then `/etc/hosts`, then enable the `qemu-ga` service. Bookends the run with `logInfo("starting")`/`logInfo("finished successfully")`; any error goes through `fatal(err)` (logs at ERROR, closes the log, `os.Exit(1)`). |
| `cloudinit.go` | `FindUserData`/`FindNetworkConfig`: glob `/dev/sr*`, mount each candidate read-only as `iso9660` in turn, read the named file off the first one that has it. |
| `userdata.go` | `UserData` struct (the Proxmox-exposed `#cloud-config` subset) + `ParseUserData` (validates the `#cloud-config` header, unmarshals YAML). |
| `apply.go` | `ApplyUserData`: `/etc/hostname` + live `sethostname(2)`, password hash via `usermod -p`, SSH authorized keys (via `writeManagedFile`, see below). |
| `network.go` | `NetworkConfig`/`NetworkConfigDevice`/`Subnet` (NoCloud `network-config` v1 subset) + `ApplyNetworkConfig`: resolves `physical` entries to a real interface by MAC (not by `name` — predictable interface naming isn't guaranteed to match what cloud-init supplied), brings interfaces up, hands DHCP/SLAAC subnet types to `dhcpcd`, applies `static`/`static6` directly via `ip addr add`/`ip route add`, merges all nameservers into one `/etc/resolv.conf` write. |
| `runit.go` | `svDir`/`runsvdirCurrent` (the runit layout: `/etc/sv/<name>` holds each service's definition, `/etc/runit/runsvdir/current/` is the active runsvdir) + `enableService`/`disableService`, both package-private: `enableService` symlinks `svDir/<name>` into `runsvdirCurrent`, mirroring `ln -s /etc/sv/<name> /etc/runit/runsvdir/current/`; `disableService` removes that symlink, mirroring `rm /etc/runit/runsvdir/current/<name>`. Both are idempotent — already-enabled/-disabled is logged and treated as success, not an error. Call sites: `network.go`'s `applyDynamicNetwork` enables `dhcpcd`, `applyStaticNetwork` disables it; `main.go` enables `qemu-ga` unconditionally. |
| `hosts.go` | `ApplyHosts`: renders `/etc/hosts` from a template; `staticAddress` picks the address to put in it (first static subnet found, else the `127.0.1.1` loopback alias). |
| `fsutil.go` | `writeManagedFile` (see below) and `withSingleTrailingNewline`. |
| `log.go` | Wires `logInfo`/`logWarn`/`logError` to `internal/vlog`, with `/var/log/void-init/void-init.log` as the file sink (see Logging section below). |
| `templates/` | `go:embed`-ed templates: `hosts` (Go `text/template`), `dhcpcd` (static, copied verbatim). |
| `testfiles/` | Sample `user-data`/`network-config` fixtures — both documentation of the supported format and test fixtures. |
| `*_test.go` | `network_test.go`, `userdata_test.go` — parse the `testfiles/` fixtures, exercise pure logic (`subnetAddressCIDR`). |

**`cmd/void-mkinitfs/`** — the host-side image-build tool; see the "`void-mkinitfs`" section below
for its architecture. **`internal/vlog/`** — the shared logger; see Logging below.

Only external dependency: `gopkg.in/yaml.v3`. Go version: 1.26.5 (see `go.mod`). Module:
`github.com/thetredev/void-init`.

`make` (see `Makefile`) builds both binaries in three modes (`debug`, `release`, `release-static`)
under `build/` (gitignored — see `.gitignore`); `go build ./...` builds both at the repo root
instead. Don't commit build artifacts either way.

## Build / test

```
go build ./...
go vet ./...
go test ./...
gofmt -l .          # should print nothing
```

Run all of these after any change before considering it done.

## Core conventions — preserve these when adding code

These describe `cmd/void-init/`; `cmd/void-mkinitfs/` follows an analogous discipline (every
external command logged before it runs via `runCommand`/`runCommandEnv` in `exec.go`, the same
error-wrapping style, no speculative abstraction) but writes to a build host rather than a live
system, so the managed-file pattern doesn't apply to it.

1. **Managed-file pattern.** Any file `void-init` writes onto a live system that a user might
   hand-edit afterward goes through `writeManagedFile` (`fsutil.go`), which regenerates everything
   up to and including the marker line `#void-init: user config starts here`, but preserves
   whatever the user appended below that marker in the file that's already on disk. Currently
   applies to `/etc/hosts`, `/etc/dhcpcd.conf`, `/etc/resolv.conf`, and
   `~/.ssh/authorized_keys`. Any new generated file that a user might reasonably want to extend by
   hand should follow the same pattern rather than a plain `os.WriteFile`.
2. **Trailing newline.** Every file `void-init` writes ends with exactly one trailing newline —
   `withSingleTrailingNewline` (`fsutil.go`) enforces this; reuse it rather than reimplementing.
3. **Error wrapping.** `fmt.Errorf("context: %w", err)` — lowercase, no trailing punctuation,
   names the path/identifier being acted on. For `os/exec` commands, wrap the command's combined
   output into the error: `fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, output)`.
4. **Comments.** Doc comments on every exported func/type explain *why*, not *what* — see any
   function in `network.go` for the model to follow. No comments inside function bodies unless
   something is genuinely non-obvious (a workaround, a hidden constraint, a surprising ordering
   requirement). Never restate what well-named identifiers already say.
5. **No speculative abstraction.** This is a deliberately small, dependency-light codebase. Don't
   introduce frameworks or libraries for things the stdlib (`os/exec`, `text/template`,
   `encoding/...`) already covers well. Three similar lines beat a premature helper.
6. **Logging discipline.** Every notable action (mount attempt, file write, service enable/disable,
   external command about to run) gets a `logInfo`/`logWarn`/`logError` call — see `apply.go`,
   `network.go`, `cloudinit.go`, `hosts.go`, `fsutil.go` for the established density/tone. **Never
   log secret material** — password hashes, private key content, or SSH key bodies. Log counts and
   usernames instead (`applyPassword`, `applySSHAuthorizedKeys` in `apply.go` are the model to
   follow).

## Logging mechanism

`internal/vlog` implements three levels (`Info`/`Warn`/`Error`, exposed as package-private
`logInfo`/`logWarn`/`logError` wrappers in each binary's own `log.go`) and a single `logf` that
formats a line in a style modeled on classic syslog (RFC3164): a timestamp, hostname,
`<program>[pid]:`, level, message — e.g. `Jul 12 10:15:23 template-vm void-init[1234]: INFO:
setting hostname to "template-vm"`. Lines are always written to stderr.

Each binary parameterizes `vlog.New(program, logPath)` differently:
- `void-init` (`cmd/void-init/log.go`) passes `/var/log/void-init/void-init.log` — a real
  boot-time system record, best-effort appended to (silently falls back to stderr-only if it can't
  be opened; a missing log file is never fatal). That file is rotated by `internal/vlog/rotate.go`
  once it reaches 50 MiB, keeping up to 5 rotated backups (`void-init.log.1` .. `void-init.log.5`,
  oldest evicted first).
- `void-mkinitfs` (`cmd/void-mkinitfs/log.go`) passes `""` — **stderr-only**, since it's an
  interactive build tool run on the host machine, not a boot-time record; a log file on the
  *build host* wouldn't mean anything.

Why not real syslog for `void-init`: it runs from `/etc/rc.local` *before* any syslog daemon (e.g.
`socklog`, which Void doesn't install by default) has started, so there's no `/dev/log` socket to
write to yet. The file-plus-stderr approach is the closest equivalent available this early in
boot.

## Domain knowledge

**Void Linux specifics:**
- Init system is `runit`, not `systemd`. A runit service is "enabled" by symlinking its
  definition from `/etc/sv/<name>` into the active `runsvdir` (`/etc/runit/runsvdir/current/`),
  and "disabled" by removing that symlink — see `enableService`/`disableService` in `runit.go`.
- `dhcpcd` is Void's standard DHCP client, also handles IPv6 SLAAC/RA (hence it covers `dhcp`,
  `dhcp4`, `dhcp6`, `ipv6_slaac`, `ipv6_dhcpv6-stateless`, `ipv6_dhcpv6-stateful` subnet types).
- Package manager is XBPS (`xbps-install`, `xbps-reconfigure`, `xbps-query`, `xbps-remove`).
  `xbps-install -r <rootdir>` can install into an arbitrary foreign root without a chroot (it just
  unpacks package files), but package pre/post-install trigger scripts (locale generation, initramfs
  builds via `dracut`, `shadow`'s user/group setup, etc.) are deferred when the target root isn't
  the host's actual `/`, and must be run afterward via `xbps-reconfigure -fa` inside something that
  furnishes a proper `/proc`/`/sys`/`/dev` — see `cmd/void-mkinitfs/nspawn.go`'s `reconfigure` for
  why `systemd-nspawn` (no `--boot`) is used for that instead of a hand-rolled chroot.
- No syslog daemon by default; `socklog` is the optional lightweight one if a user wants one, but
  it's not assumed to exist.

**Proxmox NoCloud cloud-init subset** (this project intentionally implements only what Proxmox's
Cloud-Init GUI page exposes, not the full cloud-init spec):
- `user-data`: a `#cloud-config` YAML document (magic first line required). Supported keys:
  `hostname`, `fqdn`, `manage_etc_hosts` (must be `true` for `void-init` to touch `/etc/hosts` at
  all), `user`, `password` (a hash, not plaintext — applied via `usermod -p`), `ssh_authorized_keys`.
  `disable_root` and `chpasswd.expire` are parsed but currently unused.
- `network-config`: a `version: 1` NoCloud document. `config` entries are either `type: physical`
  (a `mac_address` + list of `subnets`; matched to a real interface by MAC, not by the `name`
  field) or `type: nameserver` (a global `address`/`search` list). Subnet `type` is one of the
  DHCP/SLAAC variants above (→ `dhcpcd`) or `static`/`static6` (→ direct `ip` commands, disabling
  `dhcpcd` for that interface). All nameservers/search domains gathered across the whole config
  (static subnets + top-level `nameserver` entries) merge into a single `/etc/resolv.conf` write.
- Datasource discovery: glob `/dev/sr*` (the CD-ROM image Proxmox attaches to a VM), mount each
  candidate read-only as `iso9660` into a temp dir, stop at the first device that has the wanted
  file (`user-data` or `network-config`).

**Testing philosophy:** only pure logic gets automated tests — YAML parsing against the
`testfiles/` fixtures, `subnetAddressCIDR`'s address/CIDR math, and, in `cmd/void-mkinitfs/`,
CLI flag validation (`flags_test.go`) and pure helpers like `byUUIDSymlink` (`bootloader_test.go`).
Anything that touches the live system (mounting devices, running `ip`/`usermod`/`sgdisk`, writing
to `/etc`, `qemu-nbd`/`systemd-nspawn` invocations) is *not* unit tested; it's meant to be
exercised on an actual VM/host.

## `void-mkinitfs`

High-level facts an agent needs before touching `cmd/void-mkinitfs/`. It's fully implemented — this
is a description of what's there, not a plan for what to build:

- **Host requirement:** a `systemd`-based host with `systemd-nspawn` available (used without
  `--boot` — single-command execution with an auto-provisioned private `/proc`/`/sys`/`/dev`/`/run`,
  not a full container boot). x86_64 only, no cross-compilation, output is always a 3G qcow2.
- **CLI:** `void-mkinitfs --bios|--efi --libc=glibc|musl -o <out.qcow2>` to build from scratch, or
  `void-mkinitfs -i <existing.qcow2>` to reuse an already-bootstrapped image (layout inferred from
  partition count: 3 → BIOS, 4 → EFI; `--bios`/`--efi` becomes an optional sanity check instead of
  required in that mode).
- **Disk-first architecture:** the qcow2 is created, partitioned, and formatted *before* any
  package is installed; `xbps-install -r` targets the mounted partition stack directly — there is
  no intermediate rootfs directory to `rsync` from.
- **Partitioning via `sgdisk`** (not `parted`), GPT, explicit hex type codes: `ef02` (BIOS boot
  partition, kept on *both* layouts — matches what Proxmox VE itself does, 1M), `ef00` (EFI System
  Partition, EFI layout only, 199M `vfat -F32`), `8300` (Linux filesystem) for `/boot` (`ext2`,
  499M BIOS / 300M EFI) and `/` (`ext4`, rest of the disk).
- **Package bootstrap:** `XBPS_ARCH=x86_64|x86_64-musl xbps-install -S -R <repo> -r <mounted-root>
  <packages>`, repo chosen by `--libc` (`repo-default.voidlinux.org/current` or `.../current/musl`),
  followed by `systemd-nspawn -D <root> --resolv-conf=bind-host -- xbps-reconfigure -fa`.
- **`void-init` installation is NOT a mirror of its own boot-time logic** — just `cp` the binary
  to `/usr/local/bin/void-init` in the image and write a two-line `/etc/rc.local` that calls it.
  `sshd` generates its own host keys on first start; `void-mkinitfs` does nothing SSH-key-related.
  Only `sshd` gets enabled in the image's runsvdir — `dhcpcd` is deliberately left disabled, since
  `void-init` itself decides DHCP vs. static (and enables/disables `dhcpcd` accordingly) at first
  real boot.
- **Bootloader** (`grub-install` + `grub-mkconfig`) runs inside `systemd-nspawn --bind=/dev/nbd0`
  (nspawn's private `/dev` doesn't include host block devices by default, so this bind is
  required for `grub-install` to see the real disk). `--removable` on the EFI install, since
  there's no NVRAM to register a boot entry against at build time.
- **Cleanup:** an explicit LIFO `cleanupStack` (push a callback the moment each resource is
  acquired: nbd disconnect, each mount's unmount), unwound via one `defer` in `main()` plus a
  `SIGINT`/`SIGTERM` handler. Needed because unlike `systemd-nspawn`'s self-cleaning private mount
  namespace, loop/nbd/mount state from `qemu-nbd`/`mount` is host-visible and won't clean itself
  up on its own. **On failure, the output qcow2 is deliberately left in place** for post-mortem
  debugging, not deleted.
- **Preflight:** check `exec.LookPath` for every external tool up front, fail with one combined
  error listing everything missing, rather than dying halfway through the pipeline. If
  `xbps-install`/`xbps-reconfigure` specifically aren't found, offer to download static builds
  from `repo-default.voidlinux.org/static` into `/usr/local/bin` — that check runs last.
- **Explicitly out of scope:** cross-compilation, any output format other than qcow2, a
  non-`systemd-nspawn` (plain chroot) fallback for non-systemd hosts.
- **Package set** (`bootstrap.go`'s `packages`): `base-system`, `linux`, `dracut`, `runit-void`,
  `dhcpcd`, `iproute2`, `openssh`, `shadow`, `e2fsprogs`, `dosfstools`, `ca-certificates`,
  `iana-etc`, `bash-completion`, `net-tools`, `qemu-ga`, plus `grub-x86_64-efi` (EFI) or `grub`
  (BIOS). This list was arrived at iteratively against real builds/boots (see git history: it
  started as `base-minimal`, moved to `base-system` to get a "full" system, then gained
  `bash-completion`/`net-tools`/`qemu-ga`) rather than from a single `xbps-query -R` pass — treat
  it as working ground truth, but a future package addition should still be checked against a live
  repo before assuming the name is right.

## Documents in this repo and their role

- `README.md` — user-facing description of both shipped binaries (`void-init` and
  `void-mkinitfs`).
- `AGENTS.md` (this file) / `CLAUDE.md` — agent operating context, not user-facing; not meant to
  be linked from the README.
