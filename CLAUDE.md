# CLAUDE.md

Read **`AGENTS.md`** first — it has the general project/domain knowledge (what `void-init` is,
repo layout, code conventions, logging mechanism, cloud-init/Void Linux domain knowledge, and the
full `void-mkinitfs` summary). This file only adds Claude-Code-specific process notes for working
in this particular repo, layered on top of the global instructions already in effect.

## Commit conventions observed in this repo's history

Every commit so far in this repo (`git log`) follows a pattern worth matching if asked to commit:

- Subject-line prefixes used: `claude:` (fully Claude-authored change, e.g. the README),
  `claude/draft:` (Claude-authored draft-quality work), `claude+me:` (a collaborative
  planning/design session between the user and Claude, e.g. `void-mkinitfs.md`). If asked to
  commit, pick whichever prefix matches the nature of the change; if it's unclear which applies
  (e.g. a change that started as a Claude draft but had significant user direction), ask rather
  than guessing — the prefix is part of how this user reads their own history.
- Every commit carries a `Signed-off-by: Timo Reichl <thetredev@gmail.com>` trailer. **None** of
  them carry a `Co-Authored-By:` trailer, even though that's this harness's default commit
  behavior. Don't unilaterally decide to change trailer conventions for this repo — follow the
  standard Claude Code commit workflow unless the user gives explicit different instructions for a
  given commit. This note exists so a future commit isn't the first time this discrepancy is
  noticed.
- Subjects are short and imperative, describing *what* changed — consistent with the general style
  guidance (why belongs in doc comments when non-obvious, not repeated in the subject line).

## Working style established in this project

Observed over several planning turns on `void-mkinitfs.md`, worth carrying forward:

- The user develops `void-mkinitfs.md` as a pure planning artifact across multiple turns before
  any implementation starts. When asked to "think" or "propose a plan," write no code — and
  critically, a detailed follow-up message answering earlier open questions is *not* implicit
  permission to start implementing. Wait for an explicit go-ahead (something like "now implement
  this" or "start with step X").
- `void-mkinitfs.md` gets hand-edited by the user directly between conversation turns (typo
  fixes, wording changes, added detail). **Always re-read the file fresh with the `Read` tool
  before extending it** — don't rely on an earlier in-conversation copy or summary, and don't
  revert user edits found in it. A `<system-reminder>` noting the file changed outside the
  conversation is expected; treat the file on disk as ground truth.
- The user catches precision issues quickly (e.g. a partition-numbering off-by-one across BIOS vs.
  EFI layouts) and expects them raised back explicitly rather than silently "corrected" — when
  something in a spec looks like a typo or is ambiguous, say so and propose the fix, don't just
  quietly apply the fix you think was intended.
- Work on this repo proceeds incrementally, one reviewed step at a time (comprehensive logging,
  then the SSH `authorized_keys` marker fix, then the `void-mkinitfs` plan built up over several
  rounds) rather than as one large unreviewed change. Match that granularity when implementation
  eventually starts on `void-mkinitfs` — the plan's numbered pipeline steps are a natural unit of
  review, not something to implement all at once.
- Destructive/system-affecting actions are treated cautiously here: a request to actually run the
  built `void-init` binary was declined via this session's permission settings earlier in this
  project. Treat anything that mounts devices, changes the live hostname, writes to `/etc`, runs
  `ip`/`usermod`, or — once `void-mkinitfs` exists — touches loop/nbd devices, partitions disks
  with `sgdisk`, or runs `systemd-nspawn` against a real block device, as needing explicit
  confirmation before *executing*, not just before proposing. The user runs as root on their own
  machine and is comfortable with that, but still wants blast-radius-aware behavior from the
  agent, per the global "Executing actions with care" guidance.

## Before implementing `void-mkinitfs`

1. Re-read `void-mkinitfs.md` in full with the `Read` tool — not a cached summary — since it is
   actively edited by the user between turns.
2. Confirm scope for the specific step being implemented rather than attempting the whole
   pipeline at once (see "Working style" above).
3. Do the repo restructuring (`cmd/void-init/`, `cmd/void-mkinitfs/`, `internal/vlog/`) as its own
   reviewable step before adding any `void-mkinitfs` pipeline logic — this is called out
   explicitly in the plan's "Repo restructuring" section and in `AGENTS.md`.
4. Verify package names against a live `xbps-query -R` rather than trusting the plan's
   memory-derived package list, per the plan's own "Open items to verify" section.
5. After any code change: `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .` — this
   project has no other CI/lint tooling configured, these four commands are the full local check.

## Repo-specific tool notes

- `.claude/settings.local.json` in this repo only allowlists one narrow `Bash` pattern (an
  `xargs`/`cat` combo) — don't assume broader Bash permissions are pre-granted; most system-level
  commands relevant to this project (mount, `ip`, `usermod`, and later `qemu-nbd`/`sgdisk`/
  `systemd-nspawn`) will prompt for confirmation, as intended.
- The build artifact `void-init` at the repo root is gitignored (`.gitignore`) — don't add it or
  a future `void-mkinitfs` binary to version control. Once the `cmd/` restructuring lands, update
  `.gitignore` to match the new binary output locations.
