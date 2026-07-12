# void-init

[cloud-init](https://cloud-init.io) for Void Linux!

Well, *not really*... this project focuses heavily on getting the "Proxmox version" of cloud-init's `user-data` right*, which are those settings you can tweak in the Proxmox GUI on a VM's `Cloud-Init` page.

* At first at least - because that's what I need it for at the moment.

## What it does

`void-init` is a small, single-purpose Go binary meant to run once per boot, very early - as part of `/etc/rc.local`, which `runit` executes before any services start. On each run it:

1. Scans the configured cloud-init NoCloud datasource devices (by default `/dev/sr*`, i.e. the CD-ROM image Proxmox attaches to a VM) for a `user-data` file, mounting each candidate device read-only until one is found.
2. Parses that `user-data` as a `#cloud-config` document (the subset of keys Proxmox's Cloud-Init GUI page exposes; see [`testfiles/user-data`](testfiles/user-data)).
3. Applies it: sets the hostname, sets the target user's password hash, and installs their SSH authorized keys.
4. Looks for a `network-config` file next to `user-data` on the same datasource and, if found, parses it as cloud-init's NoCloud `network-config` v1 format (see [`testfiles/network-config`](testfiles/network-config)) and applies it: brings interfaces up, configures DHCP/SLAAC via `dhcpcd` or static addressing directly via `ip`, and writes `/etc/resolv.conf`.
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

It exits non-zero (after printing the error to stderr, prefixed with `void-init:`) on any user-data parsing/application failure. A missing `network-config` file is not an error - networking setup is simply skipped in that case.

## Building

```sh
go build .
```

This produces a `void-init` binary (see `.gitignore`) using the templates embedded at build time via `go:embed` (see [Templates](#templates)).

## Code layout

| File | Responsibility |
| --- | --- |
| [`main.go`](main.go) | Entry point; wires together finding, parsing, and applying `user-data` and `network-config`. |
| [`cloudinit.go`](cloudinit.go) | Locates the cloud-init NoCloud datasource: globs candidate devices (`/dev/sr*`), mounts each read-only in turn, and reads `user-data`/`network-config` off the first one that has it. |
| [`userdata.go`](userdata.go) | Defines the `UserData` struct (the Proxmox-exposed `#cloud-config` subset) and `ParseUserData`, which validates the `#cloud-config` header and unmarshals the YAML. |
| [`apply.go`](apply.go) | `ApplyUserData`: sets `/etc/hostname` (and the live kernel hostname), the user's password hash via `usermod`, and `~/.ssh/authorized_keys`. |
| [`network.go`](network.go) | Defines `NetworkConfig`/`NetworkConfigDevice`/`Subnet` (the NoCloud `network-config` v1 subset) and `ApplyNetworkConfig`, which brings interfaces up and configures them per subnet type; also owns `/etc/dhcpcd.conf`, `/etc/resolv.conf`, and the runit service enable/disable helpers. |
| [`hosts.go`](hosts.go) | `ApplyHosts`: renders `/etc/hosts` from the `hosts` template, and `staticAddress`, which picks the address to put in it. |
| [`fsutil.go`](fsutil.go) | Shared file-writing helpers: `writeManagedFile` (preserves the user-editable section of a managed file) and `withSingleTrailingNewline`. |
| [`templates/`](templates) | `go:embed`-ed templates for generated files (see below). |
| [`testfiles/`](testfiles) | Sample `user-data`/`network-config` fixtures, used both as documentation of the supported format and as test fixtures. |

## Supported `user-data` keys

See [`testfiles/user-data`](testfiles/user-data) for a full example. Supported keys (all optional):

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

See [`testfiles/network-config`](testfiles/network-config) for a full example. `network-config` is a `version: 1` NoCloud document whose `config` list holds a mix of:

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

On each run, `writeManagedFile` (in [`fsutil.go`](fsutil.go)) regenerates everything *up to and including* that marker, but preserves whatever the user appended after it in the file that's already on disk. This lets you hand-edit `/etc/hosts`, `/etc/dhcpcd.conf`, or `/etc/resolv.conf` below the marker without those edits being clobbered the next time void-init runs.

## Testing

```sh
go test ./...
```

Tests parse the fixtures in [`testfiles/`](testfiles) and exercise pure logic like `subnetAddressCIDR`. Nothing that touches the live system (mounting devices, running `ip`/`usermod`, writing to `/etc`) is covered by automated tests - those paths are meant to be exercised on an actual VM.

## Known limitations / TODO

- No option (yet) to generate a bootable, cloud-init-ready Void Linux rootfs from scratch with void-init pre-installed.
- Only the NoCloud datasource (CD-ROM device glob `/dev/sr*`) is supported - no HTTP/config-drive/other datasources.
