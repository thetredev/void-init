# CLAUDE.md

Read **`AGENTS.md`** first — it has the general project/domain knowledge (what `void-init` is,
repo layout, code conventions, logging mechanism, cloud-init/Void Linux domain knowledge, and the
full `void-mkinitfs` summary). This file only adds Claude-Code-specific process notes for working
in this particular repo, layered on top of the global instructions already in effect.

## Commit conventions observed in this repo's history

Every commit so far in this repo (`git log`) follows a pattern worth matching if asked to commit:

- Subject-line prefixes used: `claude:` (fully Claude-authored change, e.g. the README),
  `claude/draft:` (Claude-authored draft-quality work — the large majority of commits), `claude+me:`
  (a collaborative planning/design session between the user and Claude, e.g. the now-deleted
  `void-mkinitfs.md`). Plain, unprefixed subjects also appear (e.g. `bootstrap: add packages...`,
  `main.go: remove leftover TODO comments`) for changes the user made or drove directly rather than
  Claude authoring. If asked to commit, pick whichever prefix matches the nature of the change; if
  it's unclear which applies (e.g. a change that started as a Claude draft but had significant user
  direction), ask rather than guessing — the prefix is part of how this user reads their own
  history.
- Every commit carries a `Signed-off-by: Timo Reichl <thetredev@gmail.com>` trailer. **None** of
  them carry a `Co-Authored-By:` trailer, even though that's this harness's default commit
  behavior. Don't unilaterally decide to change trailer conventions for this repo — follow the
  standard Claude Code commit workflow unless the user gives explicit different instructions for a
  given commit. This note exists so a future commit isn't the first time this discrepancy is
  noticed.
- Subjects are short and imperative, describing *what* changed — consistent with the general style
  guidance (why belongs in doc comments when non-obvious, not repeated in the subject line).

## Working style established in this project

Originally observed over several planning turns on `void-mkinitfs.md` (a hand-edited planning doc
the user kept at the repo root while `void-mkinitfs` was being designed and built); that specific
file is gone now — it was deliberately deleted once the implementation caught up with it — but the
underlying habits it revealed still apply generally:

- Planning and implementation are kept separate turns. When asked to "think" or "propose a plan,"
  write no code — and critically, a detailed follow-up message answering earlier open questions is
  *not* implicit permission to start implementing. Wait for an explicit go-ahead (something like
  "now implement this" or "start with step X").
- Small pending-work items get tracked in a lightweight, disposable `TODO.md` at the repo root
  rather than a formal issue tracker — see e.g. the bash-prompt and reboot/poweroff-alias fixes.
  Entries get implemented one at a time (wait for the user to point at a specific item, or to say
  "implement what's in TODO.md" for what's left), and the file itself gets deleted once drained —
  don't be surprised if it's gone; that means everything in it shipped, not that it's missing.
  Files like this (and the old `void-mkinitfs.md`) can be hand-edited by the user between
  conversation turns — **always re-read them fresh with the `Read` tool before acting on them**,
  don't rely on an earlier in-conversation copy or summary, and don't revert user edits found in
  them. A `<system-reminder>` noting a file changed outside the conversation is expected; treat the
  file on disk as ground truth.
- The user catches precision issues quickly (e.g. a partition-numbering off-by-one across BIOS vs.
  EFI layouts, or an ambiguous "set PS1 in .bashrc" spec that could mean a full overwrite or an
  in-place edit) and expects them raised back explicitly rather than silently "corrected" — when
  something in a request or spec looks like a typo or is ambiguous, say so and propose options,
  don't just quietly apply the fix you think was intended.
- Work on this repo proceeds incrementally, one reviewed step at a time (comprehensive logging,
  the SSH `authorized_keys` marker fix, the `void-mkinitfs` pipeline built up step by step, the
  bash-prompt/reboot-alias fixes each landing as their own change) rather than as one large
  unreviewed change. When a task has natural sub-steps, treat each as its own reviewable unit
  rather than implementing everything at once.
- Destructive/system-affecting actions are treated cautiously here: a request to actually run the
  built `void-init` binary was declined via session permission settings earlier in this project.
  Treat anything that mounts devices, changes the live hostname, writes to `/etc`, runs
  `ip`/`usermod`, touches loop/nbd devices, partitions disks with `sgdisk`, or runs
  `systemd-nspawn` against a real block device, as needing explicit confirmation before
  *executing*, not just before proposing. The user runs as root on their own machine and is
  comfortable with that, but still wants blast-radius-aware behavior from the agent, per the
  global "Executing actions with care" guidance.

## Repo-specific tool notes

- This checkout has no `.claude/settings.local.json` — it's gitignored (`/.claude` in
  `.gitignore`), so it's per-checkout and may or may not exist depending on where you're running.
  Don't assume any particular Bash allowlist is pre-granted; if one isn't present, most
  system-level commands relevant to this project (`mount`, `ip`, `usermod`, `qemu-nbd`, `sgdisk`,
  `systemd-nspawn`) will prompt for confirmation, as intended.
- Build artifacts (`void-init`, `void-mkinitfs`, under `build/` via `Makefile` or at the repo root
  via `go build ./...`) are gitignored (`.gitignore`'s `/build`, `/void-init`, `/void-mkinitfs`
  entries) — don't add them to version control.
