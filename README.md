# void-init

[cloud-init](https://cloud-init.io) for Void Linux!

Well, *not really*... this project focuses heavily on getting the "Proxmox version" of cloud-init's `user-data` right*, which are those settings you can tweak in the Proxmox GUI on a VM's `Cloud-Init` page.

* At first at least - because that's what I need it for at the moment.

## What it does

`void-init` is a small, single-purpose Go binary meant to run once per boot, very early - as part of `/etc/rc.local`, which `runit` executes before any services start. On each run it:

1. Scans the configured cloud-init NoCloud datasource devices (by default `/dev/sr*`, i.e. the CD-ROM image Proxmox attaches to a VM) for a `user-data` file, mounting each candidate device read-only until one is found.
2. Parses that `user-data` as a `#cloud-config` document (the subset of keys Proxmox's Cloud-Init GUI page exposes; see [`cmd/void-init/testfiles/user-data`](cmd/void-init/testfiles/user-data)).
3. Applies it: sets the hostname, sets the target user's password hash, and installs their SSH authorized keys.
4. Looks for a `network-config` file next to `user-data` on the same datasource and, if found, parses it as cloud-init's NoCloud `network-config` v1 format (see [`cmd/void-init/testfiles/network-config`](cmd/void-init/testfiles/network-config)) and applies it: brings interfaces up, configures DHCP/SLAAC via `dhcpcd` or static addressing directly via `ip`, and writes `/etc/resolv.conf`.
5. Renders `/etc/hosts` from the hostname/FQDN and whatever address ended up assigned (static IP if one was configured, otherwise the `127.0.1.1` loopback alias).

Every file void-init writes is idempotent to rerun and preserves a user-editable section (see [User-editable sections](#user-editable-sections) below), and always ends with exactly one trailing newline.

## Why

Proxmox generates a NoCloud cloud-init ISO (`user-data` + `network-config`) for VMs configured via its Cloud-Init GUI page. Void Linux has no built-in cloud-init support, and installing the real `cloud-init` package pulls in a lot of machinery (Python, systemd-oriented assumptions, etc.) that doesn't fit well with a minimal, runit-based Void install. `void-init` implements just enough of that datasource/`user-data`/`network-config` handling to make Proxmox-provisioned Void VMs self-configure on first boot.

## How it's used

`void-init` is meant to be invoked from `/etc/rc.local`, which runit runs before any services are started - so it can bring up networking and apply user settings before anything else depends on them:

```sh
# /etc/rc.local
/usr/local/bin/void-init
```

It exits non-zero on any user-data parsing/application failure. A missing `network-config` file is not an error - networking setup is simply skipped in that case.

## Logging

void-init logs every notable action it takes (locating datasources, applying hostname/password/SSH keys, configuring interfaces, enabling/disabling services, writing managed files) as one line per event, in a format modeled on classic syslog (RFC3164) lines:

```
Jul 12 10:15:23 template-vm void-init[1234]: INFO: setting hostname to "template-vm"
```

Since void-init runs from `/etc/rc.local`, before any syslog daemon (e.g. `socklog`) has started, there's no `/dev/log` socket available to log to yet. Instead, log lines are written to stderr - which ends up on the console during early boot - and, best-effort, appended to `/var/log/void-init/void-init.log` so the boot's actions remain inspectable afterwards. That file is rotated once it reaches 50 MiB, keeping up to 10 rotated segments alongside it (`void-init.log.1` .. `void-init.log.10`, oldest evicted first). If the log file can't be opened (e.g. `/var` isn't writable), void-init logs to stderr only and continues; a missing log file is never fatal.

## Building

```sh
go build ./...
```

This produces both a `void-init` and a `void-mkinitfs` binary (see `.gitignore` and [void-mkinitfs](#void-mkinitfs) below) at the repo root, using the templates embedded at build time via `go:embed` (see [Templates](#templates)). To build just `void-init`: `go build ./cmd/void-init`.

## Code layout

The module builds two binaries from a `cmd/` layout, sharing `internal/vlog` (see [Logging](#logging)) as their only common code:

| File | Responsibility |
| --- | --- |
| [`cmd/void-init/main.go`](cmd/void-init/main.go) | Entry point; wires together finding, parsing, and applying `user-data` and `network-config`. |
| [`cmd/void-init/cloudinit.go`](cmd/void-init/cloudinit.go) | Locates the cloud-init NoCloud datasource: globs candidate devices (`/dev/sr*`), mounts each read-only in turn, and reads `user-data`/`network-config` off the first one that has it. |
| [`cmd/void-init/userdata.go`](cmd/void-init/userdata.go) | Defines the `UserData` struct (the Proxmox-exposed `#cloud-config` subset) and `ParseUserData`, which validates the `#cloud-config` header and unmarshals the YAML. |
| [`cmd/void-init/apply.go`](cmd/void-init/apply.go) | `ApplyUserData`: sets `/etc/hostname` (and the live kernel hostname), the user's password hash via `usermod`, and `~/.ssh/authorized_keys` (managed like the other generated files, via `writeManagedFile`). |
| [`cmd/void-init/network.go`](cmd/void-init/network.go) | Defines `NetworkConfig`/`NetworkConfigDevice`/`Subnet` (the NoCloud `network-config` v1 subset) and `ApplyNetworkConfig`, which brings interfaces up and configures them per subnet type; also owns `/etc/dhcpcd.conf`, `/etc/resolv.conf`, and the runit service enable/disable helpers. |
| [`cmd/void-init/hosts.go`](cmd/void-init/hosts.go) | `ApplyHosts`: renders `/etc/hosts` from the `hosts` template, and `staticAddress`, which picks the address to put in it. |
| [`cmd/void-init/fsutil.go`](cmd/void-init/fsutil.go) | Shared file-writing helpers: `writeManagedFile` (preserves the user-editable section of a managed file) and `withSingleTrailingNewline`. |
| [`cmd/void-init/log.go`](cmd/void-init/log.go) | Wires `void-init`'s `logInfo`/`logWarn`/`logError` to `internal/vlog`, with `/var/log/void-init/void-init.log` as the file sink. |
| [`internal/vlog/rotate.go`](internal/vlog/rotate.go) | `rotatingWriter`: rotates a `Logger`'s log file at 50 MiB, keeping up to 10 rotated segments. |
| [`cmd/void-init/templates/`](cmd/void-init/templates) | `go:embed`-ed templates for generated files (see below). |
| [`cmd/void-init/testfiles/`](cmd/void-init/testfiles) | Sample `user-data`/`network-config` fixtures, used both as documentation of the supported format and as test fixtures. |
| [`cmd/void-mkinitfs/`](cmd/void-mkinitfs) | Builds bootable Void Linux qcow2 images with `void-init` pre-installed; see [void-mkinitfs](#void-mkinitfs) below. |
| [`internal/vlog/vlog.go`](internal/vlog/vlog.go) | Shared leveled, syslog-style logger used by both binaries (see [Logging](#logging)). |

## Supported `user-data` keys

See [`cmd/void-init/testfiles/user-data`](cmd/void-init/testfiles/user-data) for a full example. Supported keys (all optional):

| Key | Effect |
| --- | --- |
| `hostname` | Written to `/etc/hostname` and applied to the running kernel via `sethostname(2)`. |
| `fqdn` | Used (together with `hostname`) when rendering `/etc/hosts`. |
| `manage_etc_hosts` | Must be `true` for void-init to touch `/etc/hosts` at all. |
| `user` | The local user password/SSH keys below are applied to. |
| `password` | A password *hash* (not plaintext), applied via `usermod -p`. |
| `ssh_authorized_keys` | List of public keys written to `~/.ssh/authorized_keys` for `user`. |

`disable_root` and `chpasswd.expire` are parsed but currently unused.

## Supported `network-config` keys

See [`cmd/void-init/testfiles/network-config`](cmd/void-init/testfiles/network-config) for a full example. `network-config` is a `version: 1` NoCloud document whose `config` list holds a mix of:

- **`type: physical`** entries - a `mac_address` plus a list of `subnets`. The entry is resolved to an actual local interface by matching `mac_address` against the host's interfaces (`name` is parsed but not used for matching, since predictable interface naming means it isn't guaranteed to match what cloud-init supplied). Each subnet is one of:
  - `dhcp`, `dhcp4`, `dhcp6`, `ipv6_slaac`, `ipv6_dhcpv6-stateless`, `ipv6_dhcpv6-stateful` - the interface is brought up and handed to `dhcpcd` (which handles both DHCP and IPv6 SLAAC/RA), enabling the `dhcpcd` runit service.
  - `static`, `static6` - the interface is brought up and addressed directly via `ip addr add`/`ip route add`, and the `dhcpcd` service is disabled for it. `address` may either be a plain address with a separate dotted-decimal `netmask`, or carry its own `/<prefix>` suffix (e.g. `fd8c::1/64`), which takes precedence. `gateway` is optional; if set, a default route is added. `dns_nameservers`/`dns_search` contribute to `/etc/resolv.conf`.
- **`type: nameserver`** entries - a global `address` list and `search` list, merged into `/etc/resolv.conf` alongside anything gathered from static subnets.

All nameservers/search domains gathered across the whole config (from both static subnets and top-level `nameserver` entries) are merged into a single `/etc/resolv.conf` write.

## Templates

Two files are `go:embed`-ed at build time and rendered/copied at runtime:

- [`templates/hosts`](templates/hosts) - a Go `text/template` for `/etc/hosts`, substituting `{{.Address}}`, `{{.FQDN}}`, and `{{.Hostname}}`.
- [`templates/dhcpcd`](templates/dhcpcd) - a static `dhcpcd.conf` written verbatim for any interface configured via DHCP/SLAAC.

## User-editable sections

Every managed file void-init writes contains the marker line:

```
#void-init: user config starts here
```

On each run, `writeManagedFile` (in [`fsutil.go`](fsutil.go)) regenerates everything *up to and including* that marker, but preserves whatever the user appended after it in the file that's already on disk. This lets you hand-edit `/etc/hosts`, `/etc/dhcpcd.conf`, `/etc/resolv.conf`, or `~/.ssh/authorized_keys` below the marker without those edits being clobbered the next time void-init runs.

## Testing

```sh
go test ./...
```

Tests parse the fixtures in [`cmd/void-init/testfiles/`](cmd/void-init/testfiles) and exercise pure logic like `subnetAddressCIDR`. Nothing that touches the live system (mounting devices, running `ip`/`usermod`, writing to `/etc`, or - for `void-mkinitfs` - `qemu-nbd`/`sgdisk`/`systemd-nspawn`) is covered by automated tests - those paths are meant to be exercised on an actual VM/host.

## `void-mkinitfs`

`void-mkinitfs` is a separate, host-side build tool that produces a bootable, cloud-init-ready Void Linux qcow2 disk image with `void-init` pre-installed and `/etc/rc.local` wired up to run it, so a VM booted from that image self-configures via `void-init` on first boot. It runs on a `systemd`-based host (uses `systemd-nspawn`, without `--boot`, to run package post-install scripts and install the bootloader inside the image being built) and targets x86_64 only.

```sh
# Build a new 3G qcow2 from scratch, BIOS or UEFI:
void-mkinitfs --bios --libc=glibc -o void.qcow2
void-mkinitfs --efi  --libc=musl  -o void.qcow2

# Reuse an already-built image to refresh void-init/rc.local without
# re-bootstrapping packages (layout is inferred from partition count):
void-mkinitfs -i void.qcow2

# Overwrite an existing output file, and don't prompt before downloading
# xbps tools/keys (unattended/scripted runs):
void-mkinitfs --bios -o void.qcow2 -f -y
```

Full design/implementation details - partition layout, package set, the `xbps-install`/`systemd-nspawn` pipeline, cleanup/error-handling strategy - live in [`void-mkinitfs.md`](void-mkinitfs.md).

Requires `xbps-install`, `xbps-reconfigure`, `systemd-nspawn`, `qemu-img`, `qemu-nbd`, `sgdisk`, `mkfs.vfat`, `mkfs.ext2`, `mkfs.ext4`, `partprobe`, `udevadm`, `blkid`, `grub-install`, and `grub-mkconfig` on `PATH`; run as root. If `xbps-install`/`xbps-reconfigure` or Void's repository signing keys aren't found locally, `void-mkinitfs` offers to download and checksum-verify them from Void's live static archive into `/usr/local/bin`/`/usr/local/share/void-mkinitfs/keys` (`-y`/`--yes` skips the confirmation, `--update-xbps` forces a refresh).

## Known limitations / TODO

- Only the NoCloud datasource (CD-ROM device glob `/dev/sr*`) is supported - no HTTP/config-drive/other datasources.
- `void-mkinitfs` targets x86_64 only (no cross-compilation), qcow2 output only, and requires a `systemd`-based host (no plain-chroot fallback for non-`systemd` hosts yet) - see `void-mkinitfs.md`'s "Explicitly out of scope" section.
