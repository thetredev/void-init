# `void-mkinitfs` — implementation plan

This document is a design/implementation plan only. Nothing described here is implemented yet.

## Goal

Implement TODO #2 from `main.go`: a second binary, `void-mkinitfs`, that builds a bootable, cloud-init-ready Void Linux qcow2 disk image from scratch, with `void-init` pre-installed and wired into `/etc/rc.local`. Runs on only on `systemd`-based hosts (for now, requires `systemd-nspawn`), targets x86_64 only, no cross-compilation.

## Repo restructuring

The module currently builds a single binary from a flat `package main` at the repo root. Two
binaries requires moving to a `cmd/` layout:

```
go.mod, go.sum                     # unchanged, one module for both binaries
internal/
  vlog/
    vlog.go                        # shared leveled-logger package (see Logging)
cmd/
  void-init/
    main.go, apply.go, cloudinit.go, network.go, hosts.go, userdata.go, fsutil.go, *_test.go
    templates/hosts, templates/dhcpcd
    testfiles/user-data, testfiles/network-config
  void-mkinitfs/
    main.go                        # flag parsing + orchestration + cleanup stack
    bootstrap.go                   # xbps-install invocation
    image.go                       # qemu-img/qemu-nbd/sgdisk/mkfs/mount helpers
    nspawn.go                      # systemd-nspawn wrapper
    fstab.go                       # UUID lookup + /etc/fstab rendering
    install.go                     # void-init binary + rc.local
    bootloader.go                  # grub-install / grub-mkconfig
    templates/fstab
```

`go:embed` patterns are relative to the embedding file's directory and can't reference `..`, so `templates/`/`testfiles/` move under `cmd/void-init/` verbatim (no source changes needed beyond the move). `void-mkinitfs` gets its own small `templates/fstab` for the file described below.

`log.go`'s current content (the leveled `logInfo`/`logWarn`/`logError` helpers, RFC3164-style line format) moves into `internal/vlog` as a proper package so both binaries share the exact same log format and level semantics. Each `cmd/*/main.go` keeps a thin package-level `logInfo`/`logWarn`/`logError` wrapping a package-level `*vlog.Logger` instance, so none of the existing `void-init` call sites change:

```go
// cmd/void-init/log.go
var logger = vlog.New("void-init", "/var/log/void-init.log")

func logInfo(format string, args ...any)  { logger.Info(format, args...) }
func logWarn(format string, args ...any)  { logger.Warn(format, args...) }
func logError(format string, args ...any) { logger.Error(format, args...) }
```

```go
// cmd/void-mkinitfs/log.go
var logger = vlog.New("void-mkinitfs", "") // empty path = stdout/stderr depending on log level
```

`vlog.New(program, logPath string) *Logger`: if `logPath == ""`, only writes to stdout/stderr. If non-empty, best-effort opens/appends that file too (falls back to stderr-only with a warning if it can't be opened, same as today). This preserves `void-init`'s current behavior exactly and gives `void-mkinitfs` stderr-only logging as agreed (it's an interactive CLI build tool run on the host, not a boot-time system record — `/var/log/void-init.log` on the *build host* wouldn't mean anything).

## `void-mkinitfs` CLI

```
void-mkinitfs --bios|--efi --libc=glibc|musl -o <output.qcow2>
void-mkinitfs -i <image.qcow2>
```

- `--bios` / `--efi`: mutually exclusive. Selects both the bootloader package/install target and
  the partition layout. Required when building from scratch (`-o`); **not** required with `-i`,
  where the layout is instead inferred from the attached image's partition count — see step 10.
  Still accepted alongside `-i` as an optional override/sanity check (`void-mkinitfs` fails if it
  conflicts with what the partition count implies).
- `--libc`: `glibc` (default) or `musl`. Selects the XBPS repository path and `XBPS_ARCH`. Not
  used with `-i` (no packages are bootstrapped).
- `-o` / `--output`: destination qcow2 path. Fails if the file already exists (no silent
  overwrite — this is a destructive-adjacent, host-affecting tool, not an idempotent one like
  `void-init` itself) unless `-f`/`--force` is also given.
- `-i` / `--image <qcow2 path>`: reuse an existing qcow2 instead of building a rootfs from
  scratch — see step 10. Mutually exclusive with `-o`: `-i` operates on the given image in
  place, it doesn't produce a separate output file.
- `-f` / `--force`: with `-o`, remove an existing file at the output path (right before
  `qemu-img create`, not during flag validation) instead of failing. Only applies alongside `-o`
  — rejected alongside `-i`, which has no output file to overwrite.
- `--update-xbps`: force a re-download/re-verify of the cached xbps tools (`/usr/local/bin`) and
  repository signing keys (`/usr/local/share/void-mkinitfs/keys`) from Void's live static archive,
  even if both are already present. Only applies when building from scratch — rejected alongside
  `-i`, since `-i` never bootstraps packages and so never touches either (see "Repository keys"
  below).
- `-y` / `--yes`: assume yes to the "download and verify Void's static xbps tools/keys?"
  confirmation preflight asks before fetching from the live static archive (see "Repository keys"
  below), for unattended/scripted runs.

Preflight, before doing anything else: check `exec.LookPath` for every external tool the pipeline needs (`xbps-install`, `xbps-reconfigure`, `systemd-nspawn`, `qemu-img`, `qemu-nbd`, `sgdisk`, `mkfs.vfat`, `mkfs.ext2`, `mkfs.ext4` `partprobe`, `udevadm`, `blkid`, `grub-install`, `grub-mkconfig`) and fail with one clear error listing everything missing, rather than dying halfway through the pipeline on the first missing tool. If `xbps-install`/`xbps-reconfigure` are not available in `/usr/local/bin` or on `PATH`, or the repository key cache (`/usr/local/share/void-mkinitfs/keys`) is empty, `void-mkinitfs` will ask for permission to download and checksum-verify Void's static tools/keys from [https://repo-default.voidlinux.org/static](https://repo-default.voidlinux.org/static) — see "Repository keys" below. That check runs last, and is skipped entirely with `-i`.

### Repository keys

`xbps-install`'s key trust is scoped per-rootdir (`<rootdir>/var/db/xbps/keys/`), not host-global,
and step 5 always targets a freshly created rootdir (the just-mounted partition stack) that has
never trusted anything — so without pre-seeding it, `xbps-install` blocks on an interactive
"import this public key?" prompt it can't read an answer to (`void-mkinitfs` isn't attached to a
TTY), even with `-y`. Void doesn't publish a separate `archlinux-keyring`-style package for this:
the trusted repository signing key(s) ship bundled inside `xbps` itself, and inside the static
tarball at `var/db/xbps/keys/*.plist`.

`void-mkinitfs` doesn't hardcode any key. Instead, whenever it needs to (re)provision the static
xbps tools — missing from `/usr/local/bin`, missing keys in the local cache, or `--update-xbps` —
it downloads [`sha256sums.txt`](https://repo-default.voidlinux.org/static/sha256sums.txt) alongside
the tarball, verifies the tarball's sha256 digest is one `sha256sums.txt` actually lists, and only
then extracts `var/db/xbps/keys/*.plist` from the verified tarball into a local cache
(`/usr/local/share/void-mkinitfs/keys`). A checksum mismatch is fatal — nothing extracted from an
unverified tarball is installed or trusted. Before every step-5 bootstrap, that cache is copied
into `<tmp>/var/db/xbps/keys/`, mirroring what `void-installer` does when seeding a fresh target
root from the host's own trusted keys.

Caveat found at implementation time: `sha256sums.txt`'s row for the `-latest` alias filename
itself (`xbps-static-latest.x86_64-musl.tar.xz`) was observed to lag behind what that alias
currently serves — Void's build infra doesn't appear to regenerate the alias's own checksum row on
every version bump, even though the real per-version row (e.g.
`xbps-static-static-0.60.4_1.x86_64-musl.tar.xz`) is correct. Verification therefore checks whether
the downloaded digest appears *anywhere* in `sha256sums.txt`, rather than looking up the row for
the alias filename specifically — this still only accepts bytes Void's own build actually published
and recorded a checksum for, it just doesn't assume the alias's own bookkeeping is current.

This step is skipped entirely with `-i`: reusing an existing image never runs step 5 (no packages
are bootstrapped), so it never needs repository keys either.

## Cleanup strategy

Once the qcow2 file is created and `qemu-nbd` connects it, most of the pipeline's state is host-visible (loop/nbd device, mounts) and *not* self-cleaning like `systemd-nspawn`'s private mount namespace is. A single ordered cleanup stack is used instead of ad-hoc `defer`s scattered across files, since steps are pushed onto it dynamically as the pipeline progresses and must be unwound in strict reverse order on both success and failure:

```go
type cleanupStack struct{ fns []func() }

func (c *cleanupStack) push(fn func())  { c.fns = append(c.fns, fn) }
func (c *cleanupStack) unwind() {
    for i := len(c.fns) - 1; i >= 0; i-- {
        c.fns[i]()
    }
}
```

`main()` does `defer cleanup.unwind()` once, and a `signal.Notify` handler for `SIGINT`/`SIGTERM` also triggers `unwind()` before exiting, so a Ctrl-C mid-run still detaches the nbd device and unmounts partitions instead of leaking that state on the host.

On failure, the cleanup stack unmounts/disconnects everything, but the **output qcow2 file itself is left in place**, not deleted — a half-built image is more useful for post-mortem debugging than silently vanishing.

## Pipeline

### 1. Create the qcow2 and attach it via NBD

```
qemu-img create -f qcow2 <output> 3G
modprobe nbd            # ensure the module is loaded; kernel auto-creates nbd0p* on partprobe
qemu-nbd -c /dev/nbd0 <output>
```

Push `qemu-nbd -d /dev/nbd0` onto the cleanup stack immediately after a successful connect. v1 hardcodes `/dev/nbd0` (single sequential run, not concurrent) and fails loudly if it's already in use rather than scanning for a free device — that's a possible future improvement, not needed now.

`qemu-nbd -c`/`-d` both fork and return before the kernel has necessarily finished negotiating the device with them — `sgdisk` (step 2) can hit `/dev/nbd0` while the kernel still reports it as 0 sectors, corrupting the partition table it "creates" there. After both `-c` and `-d`, poll `/sys/class/block/nbd0/size` (0 while disconnected, nonzero once attached) until it reflects the expected state, bounded by a timeout, instead of trusting the command's exit status alone.

### 2. Partition with `sgdisk`

GPT on both layouts!

**`--bios`:**

| # | Size | Type | Mount |
|---|------|------|-------|
| 1 | 1M | `ef02` (BIOS boot) | — |
| 2 | 499M | `8300` (Linux filesystem), `ext2` | `/boot` |
| 3 | rest | `8300`, `ext4` | `/` |

```
sgdisk -o /dev/nbd0
sgdisk -n 1:0:+1M   -t 1:ef02 -c 1:"BIOS boot" /dev/nbd0
sgdisk -n 2:0:+499M -t 2:8300 -c 2:"boot"      /dev/nbd0
sgdisk -n 3:0:0     -t 3:8300 -c 3:"root"      /dev/nbd0
```

**`--efi`:**

| # | Size | Type | Mount |
|---|------|------|-------|
| 1 | 1M | `ef02` (BIOS boot) | — |
| 2 | 199M | `ef00` (EFI System), `vfat -F32` | `/boot/efi` |
| 3 | 300M | `8300`, `ext2` | `/boot` |
| 4 | rest | `8300`, `ext4` | `/` |

```
sgdisk -o /dev/nbd0
sgdisk -n 1:0:+1M   -t 1:ef02 -c 1:"BIOS boot" /dev/nbd0
sgdisk -n 2:0:+199M -t 2:ef00 -c 2:"EFI"       /dev/nbd0
sgdisk -n 3:0:+300M -t 3:8300 -c 3:"boot"      /dev/nbd0
sgdisk -n 4:0:0     -t 4:8300 -c 4:"root"      /dev/nbd0
```

The `ef02` BIOS-boot partition is kept on the `--efi` layout too, as it matches what Proxmox VE does to sidestep firmware/boot-layout edge cases at the cost of just 1M.

After partitioning, run `partprobe /dev/nbd0` (kernel needs to be told to re-read the table before `/dev/nbd0p1`../pN` device nodes reflect it) before touching those nodes. `partprobe` only triggers that kernel re-read and returns — the `/dev/nbd0pN` nodes are then created asynchronously by udev processing the resulting uevents, so also run `udevadm settle` right after it (same race applies to step 10's partition-count detection, which runs `partprobe` too) before touching those nodes.

### 3. Filesystems

```
# --bios
mkfs.ext2 -L boot /dev/nbd0p2
mkfs.ext4 -L root /dev/nbd0p3

# --efi
mkfs.vfat -F32 -n EFI  /dev/nbd0p2
mkfs.ext2      -L boot /dev/nbd0p3
mkfs.ext4      -L root /dev/nbd0p4
```

### 4. Mount, root first, then nested

```
mount /dev/nbd0pN <tmp>              # root partition -> <tmp>
mkdir <tmp>/boot
mount /dev/nbd0pM <tmp>/boot         # boot partition -> <tmp>/boot

# --efi only:
mkdir <tmp>/boot/efi
mount /dev/nbd0p2 <tmp>/boot/efi     # ESP -> <tmp>/boot/efi
```

Each successful mount pushes its unmount onto the cleanup stack immediately, so partial mount failures still unwind correctly. Unmount order is strictly the reverse: `efi`, then `boot`, then `root`.

### 5. Bootstrap packages directly onto the mounted target (architecture: option b)

There's no intermediate rootfs directory — `xbps-install` targets the mounted partition stack (`<tmp>`) directly with `-r`, avoiding a redundant copy/rsync pass and doubled disk usage.

```
XBPS_ARCH=<x86_64|x86_64-musl> xbps-install -S -y \
  -R <repo-for-libc> \
  -r <tmp> \
  <package set>
```

`-y` is required because `void-mkinitfs` isn't attached to a TTY: on the first fetch against a repo whose signing key isn't already trusted, `xbps-install` otherwise blocks on an interactive "import this public key?" prompt it can't read an answer to. In practice `-y` alone doesn't cover that specific prompt: XBPS's key trust is scoped per-rootdir (`<rootdir>/var/db/xbps/keys/`), and `<tmp>` here is always a freshly created rootdir that has never trusted anything, so the prompt fires on *every* build. `void-mkinitfs` pre-seeds `<tmp>/var/db/xbps/keys/` before running `xbps-install` to avoid it — see "Repository keys" above for how those keys are sourced (no hardcoded/embedded key; always from Void's live static archive, checksum-verified).

Repo/arch by `--libc`:
- `glibc` (default): `XBPS_ARCH=x86_64`, repo `https://repo-default.voidlinux.org/current`
- `musl`: `XBPS_ARCH=x86_64-musl`, repo `https://repo-default.voidlinux.org/current/musl`

**Proposed package set** (names to double-check against the live repo at implementation time — going from memory, not a verified snapshot):

- Common: `base-minimal`, `linux`, `dracut`, `runit-void`, `dhcpcd`, `iproute2`, `openssh`,
  `shadow`, `e2fsprogs`, `dosfstools`, `ca-certificates`, `iana-etc`
- `--bios` adds: `grub`
- `--efi` adds: `grub-x86_64-efi`

`xbps-install -r` with a foreign root just unpacks package files — it doesn't run pre/post install scriptlets whose target differs from the host's actual `/`, which is exactly the gap `systemd-nspawn` fills in the next step.

### 6. Reconfigure via `systemd-nspawn` (no `--boot`)

```
systemd-nspawn -D <tmp> --resolv-conf=bind-host -- xbps-reconfigure -fa
```

Runs every deferred package trigger (initramfs generation via `dracut` for the just-installed kernel, `shadow`'s user/group setup, locale generation, ca-certificates bundle, etc.) inside a properly furnished root — nspawn sets up `/proc`, `/sys`, a private `/dev`, and `/run` as tmpfs automatically, and tears all of it down itself when the command exits, success or failure. No manual `mount --bind` bookkeeping, nothing host-visible to leak.

### 7. `/etc/fstab`

After `mkfs` (step 3), read each partition's UUID via `blkid` and render a small embedded template (`cmd/void-mkinitfs/templates/fstab`) into `<tmp>/etc/fstab`, listing root, `/boot`, and (for `--efi`) `/boot/efi` by `UUID=`.

### 8. Install `void-init` and wire up `rc.local`

No mirroring of `void-init`'s own boot-time logic:

```
cp void-init <tmp>/usr/local/bin/void-init
chmod 0755 <tmp>/usr/local/bin/void-init
```

Write `<tmp>/etc/rc.local`:

```sh
#!/bin/sh
/usr/local/bin/void-init
```

`sshd` (already installed, package set above) generates its own host keys on first start via its own runit service — `void-mkinitfs` does nothing SSH-key-related.

`runit-void`'s default service dir needs `sshd` and `dhcpcd`'s *absence* handled correctly: `dhcpcd` should **not** be enabled by default here (`void-init`'s own `ApplyNetworkConfig` at first real boot decides DHCP vs. static and enables/disables it itself, per `network.go`) — only `sshd` gets enabled by symlinking into the image's default runsvdir, mirroring what `enableService` in `network.go` does, but invoked directly against `<tmp>` rather than the live system.

### 9. Bootloader, also via `systemd-nspawn`

`grub-install` needs to see the real block device (`/dev/nbd0`) to write the boot sector / determine the device map — nspawn's private `/dev` doesn't include host block devices by default, so this step needs explicit bind mounts, of both `/dev/nbd0` itself *and every partition node* (`/dev/nbd0p1`, `p2`, ...). `grub-probe` canonicalizes the specific device backing `/boot`'s mount (`/dev/nbd0pN`, per `/proc/self/mountinfo`) while determining the device map, and fails with "failed to get canonical path" if that node isn't present too — binding only the parent disk isn't enough:

```
# --bios
systemd-nspawn -D <tmp> --bind=/dev/nbd0 --bind=/dev/nbd0p1 --bind=/dev/nbd0p2 --bind=/dev/nbd0p3 -- \
  grub-install --target=i386-pc --boot-directory=/boot /dev/nbd0

# --efi
systemd-nspawn -D <tmp> --bind=/dev/nbd0 --bind=/dev/nbd0p1 --bind=/dev/nbd0p2 --bind=/dev/nbd0p3 --bind=/dev/nbd0p4 -- \
  grub-install --target=x86_64-efi --efi-directory=/boot/efi --boot-directory=/boot --removable
```

`--removable` on the EFI install writes the fallback `\EFI\BOOT\BOOTX64.EFI` path instead of registering an NVRAM boot entry via `efibootmgr` — this image isn't running on real firmware at build time, so there's no NVRAM to register against; `--removable` is what makes it bootable "as-is" once attached to a VM.

Then, a follow-up nspawn invocation with the *same* bind mounts as above — `grub-mkconfig` also runs `grub-probe`, this time against `/` (`/dev/nbd0p3` on `--bios`), so it needs the partition nodes visible too, not just the first invocation:

```
systemd-nspawn -D <tmp> --bind=/dev/nbd0 --bind=/dev/nbd0p1 --bind=/dev/nbd0p2 --bind=/dev/nbd0p3 -- \
  grub-mkconfig -o /boot/grub/grub.cfg
```

### 10. `-i <qcow2 path>`: reuse an existing image instead of building a new one

`void-mkinitfs -i <path>` skips rootfs creation entirely — no `qemu-img create` (step 1), no partitioning (step 2), no filesystem creation (step 3), no package bootstrap (step 5), no `xbps-reconfigure` (step 6), no `/etc/fstab` render (step 7) — and instead attaches the given qcow2 directly via `qemu-nbd` (the attach half of step 1, without the `create`), mounts its partitions per step 4, and runs only step 8 (install/refresh `void-init` and `rc.local`) and, if
requested, step 9 (bootloader) against it. This exists for fast dev-loop iteration on `void-init` itself: rebuild the boot-time binary and drop it into an already-bootstrapped image without re-running `xbps-install`/`xbps-reconfigure`, by far the slowest part of the pipeline, on every iteration.

**Layout inference instead of `--bios`/`--efi`.** Unlike the from-scratch path, `-i` doesn't require `--bios`/`--efi` to know which layout it's dealing with — after `qemu-nbd -c` attaches the image and `partprobe /dev/nbd0` settles, `void-mkinitfs` counts the partition device nodes that show up (`/dev/nbd0p1`, `p2`, ... — e.g. via `lsblk -no NAME /dev/nbd0` or by globbing `/sys/class/block/nbd0/nbd0p*`) and maps the count directly onto the two layouts from step 2, since they're deliberately distinguishable by partition count alone:

| Partitions found | Layout | Mounts applied (step 4) |
|---|---|---|
| 3 | `--bios` | p2 → `/boot`, p3 → `/` |
| 4 | `--efi` | p2 → `/boot/efi`, p3 → `/boot`, p4 → `/` |
| anything else | error | `void-mkinitfs` refuses to guess and exits with an error naming the count it saw |

This is enough to disambiguate because the two layouts this tool ever produces differ in partition *count*, not just content — there's no case where a 3-partition and a 4-partition image both need the same treatment, so count alone is a reliable discriminator without needing to inspect partition type GUIDs or filesystem types at all. (If `--bios`/`--efi` is also passed alongside `-i` as an explicit override/sanity check, `void-mkinitfs` compares it against the inferred layout and fails loudly on a mismatch rather than silently trusting one over the other.)

This inference still rests on the same assumption as before: **`-i` assumes the image has exactly one of the two partition layouts described in step 2** — three or four partitions in the specific order/purpose (`bios_grub`, [`ESP`,] `boot`, `root`) documented there. It counts partitions to pick *which* of those two known layouts applies, but doesn't otherwise validate partition types, filesystem types, or sizes. Pointing `-i` at a qcow2 with some other 3- or 4-partition scheme that wasn't produced by `void-mkinitfs` will mount the wrong partition at the wrong path silently.

### 11. Teardown

Cleanup stack unwinds in reverse: unmount `/boot/efi` (if present), unmount `/boot`, unmount `/`, `partprobe`/settle isn't needed on teardown, `qemu-nbd -d /dev/nbd0`. Output qcow2 is done.

## Open items to verify during implementation (not blocking this plan)

- Exact current package names in the void repo (`base-minimal` contents, whether `grub` alone   covers `i386-pc` target or needs a differently-named package, etc.) — confirm against a live   `xbps-query -R` at implementation time rather than trusting memory.
- Whether `runit-void`'s default runsvdir needs anything beyond symlinking `sshd` in, or whether additional base wiring (e.g. `agetty`/console service) is expected out of the box.
- `qemu-nbd`/`nbd` kernel module availability/permissions on the user's host machine (or a dedicated VM) with `root` privileges

## Explicitly out of scope for this iteration

- Cross-compilation / non-x86_64 targets.
- Any output format other than qcow2.
- A non-`systemd-nspawn` (plain chroot) fallback for non-systemd hosts — deferred until the nspawn path is implemented and working, as you originally scoped it.
