1. Code optimization

**Applied (2 fixes):**
- `cmd/void-mkinitfs/cleanup.go` — fixed a real data race: `main`'s deferred `unwind()` and the signal-handler goroutine's `unwind()` could run concurrently, double-running cleanups (double `umount`/`qemu-nbd -d`) or letting the process exit mid-unmount. `cleanupStack` now has a mutex; a second caller blocks until the first finishes, then finds an empty stack.
- `cmd/void-mkinitfs/image.go` — removed a dead `sort.Strings(matches)` in `detectLayout` (the glob results are only counted, never read in order) and the now-unused `sort` import.

**Considered and deliberately left alone** (this codebase is exec-bound — its runtime is dominated by `xbps-install`/`systemd-nspawn`/`mkfs`, so micro-optimizations buy nothing):
- `vlog` calls `os.Hostname()` on every log line. Caching it would be wrong: `void-init` changes the hostname mid-run, and hostname-at-log-time is correct syslog behavior.
- `partition()` spawns one `sgdisk` per partition; merging into one invocation would save 2–3 process spawns in a pipeline that takes minutes — not worth losing the per-partition log lines.
- `runCommandEnv` buffers full command output for error context; a failing `xbps-install` duplicates megabytes into the error string. Capping the buffer to the last few KiB is a reasonable future tweak, but it's error-path-only.
- Readability and comment quality are already strong and consistent (why-comments, no restating, no speculative abstraction). The `writeFile`/`copyFile` duplication between the two binaries is deliberate per the "only `vlog` is shared" rule. No churn justified.
- Optional UX: `preflight` could check `os.Geteuid() == 0` up front for a clearer error than `mkfs`/`mount` failing later.

## 2. Security audit

**Permissions assertion — all correct.** Every file write was checked:

| File | Mode | Verdict |
|---|---|---|
| `~/.ssh` / `authorized_keys` | 0700 / 0600 + `chown` to user | ✅ matches sshd expectations |
| `/etc/hostname`, `/etc/hosts`, `/etc/resolv.conf`, `/etc/dhcpcd.conf`, image `/etc/fstab` | 0644 | ✅ standard, no secrets |
| Image `/etc/rc.local`, `/usr/local/bin/void-init`, host xbps tools | 0755 root-owned | ✅ not group/world-writable (critical for rc.local, which runit runs as root) |
| Image `/root/.bashrc`, `/root/.profile` | 0644 inside `/root` (0700 from base-files) | ✅ effectively root-only |
| `/etc/skel/.profile`, `bashrc.d/*.sh` | 0644, dirs 0755 | ✅ root-owned, sourced not executed |
| Repo keys (image + host cache) | 0644 | ✅ public material |
| Log file / dir | 0644 / 0755 | ✅ safe because the no-secrets-in-logs convention is honored everywhere (verified every log call: hash and key bodies never logged) |
| Temp dirs/files | `MkdirTemp`/`CreateTemp` → 0700/0600 | ✅ |

Also verified: no shell string interpolation anywhere in `void-init` (all `exec.Command` argv); repo keys are pre-seeded so xbps signature verification is active from the first package; download refuses anything not in `sha256sums.txt`.

**Findings, ranked:**
1. **Untrusted-image caveat (design note, not a bug):** `-i --reinstall-bootloader` runs `grub-install` *from the image* as root via nspawn (no user namespacing), and any `-i` mounts the image's ext4 (kernel parsing surface). Fine for the intended self-built images; worth one README sentence: "only point `-i` at images you built."
2. **`usermod -p <hash>`** exposes the hash in `/proc/*/cmdline` while usermod runs. Very low severity here (early boot, before any login is possible, hash not plaintext), and easily fixed by piping `user:hash` into `chpasswd -e` via stdin if you ever care.
3. **Argv hardening:** a `user:` value starting with `-` would be parsed by usermod as a flag. Fail-safe (usermod errors out), but adding `--` before the username costs nothing.
4. **Trust model of the static-tools download:** sha256sums.txt comes from the same origin as the tarball, so verification protects integrity, not against a compromised server; the extracted keys are then TOFU'd as the package trust root. Documented in the code and reasonable — just stating it explicitly.
5. **`http.Get` has no timeout** — a stalled server hangs a build forever. Robustness fix: an `http.Client` with a generous timeout.
6. Completeness only: `grubMkconfigCommand` interpolates blkid UUIDs into `sh -c` — safe (ext UUIDs are canonical hex) and moot given finding 1; `copyFile`'s perm applies only at creation (`O_CREATE|O_TRUNC` keeps an existing file's mode — currently always the same value, but a footgun if reused); `os.WriteFile` modes are umask-masked (with root's 022 they're exact; a stricter umask only ever tightens, never loosens).

Given that neither binary runs as a service and every write is root-owned with correct modes, the practical attack surface reduces to the two caveats in findings 1–2.
