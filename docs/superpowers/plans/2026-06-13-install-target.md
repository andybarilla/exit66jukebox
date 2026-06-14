# Install Target Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `make install` that builds the jukebox and installs it as a per-user systemd service on Arch or Fedora, with XDG path defaults and an env file for overrides/secrets.

**Architecture:** Two static template files in a new `packaging/` directory (systemd unit + env example) plus three new Makefile targets (`check-prereqs`, `install`, `uninstall`). Install copies files under `$HOME` (`~/.local/bin`, `~/.config`, `~/.local/share`), never clobbers an existing env file, and runs `systemctl --user daemon-reload`. No root, no auto-enable, no native packaging.

**Tech Stack:** GNU Make, POSIX sh (Makefile recipes), systemd user units.

Spec: `docs/superpowers/specs/2026-06-13-install-target-design.md`

---

## File Structure

- Create: `packaging/exit66jukebox.service` — systemd user unit template (uses `%h` specifiers, resolved by systemd at runtime; copied verbatim).
- Create: `packaging/exit66.env.example` — env file template installed to `~/.config/exit66jukebox/exit66.env`.
- Modify: `Makefile` — add `check-prereqs`, `install`, `uninstall` targets and add them to `.PHONY`.
- Modify: `README.md` — add a short "Install" section pointing at `make install`.

Manual verification only (Makefile/shell glue, no Go code), so no Go test files. Each task ends with a concrete command + expected output.

---

### Task 1: systemd unit template

**Files:**
- Create: `packaging/exit66jukebox.service`

- [ ] **Step 1: Write the unit file**

Create `packaging/exit66jukebox.service` with exactly this content. `%h` is the
systemd specifier for the user's home dir, resolved when the unit runs — do not
expand it at install time. `$EXIT66_ARGS` is intentionally unbraced so systemd
word-splits it into separate args.

```ini
[Unit]
Description=Exit 66 Jukebox
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%h/.local/bin/exit66jukebox -db %h/.local/share/exit66jukebox/exit66.db $EXIT66_ARGS
EnvironmentFile=%h/.config/exit66jukebox/exit66.env
Restart=on-failure

[Install]
WantedBy=default.target
```

- [ ] **Step 2: Verify it parses**

Run: `systemd-analyze verify --user packaging/exit66jukebox.service; echo exit=$?`
Expected: `exit=0`. (A warning that the ExecStart binary does not yet exist is
acceptable — it must not report a syntax error. If `systemd-analyze` is
unavailable, skip and rely on Task 5's live load.)

- [ ] **Step 3: Commit**

```bash
git add packaging/exit66jukebox.service
git commit -m "feat(packaging): add systemd user unit template"
```

---

### Task 2: env file template

**Files:**
- Create: `packaging/exit66.env.example`

- [ ] **Step 1: Write the env template**

Create `packaging/exit66.env.example` with exactly this content. The `%h/Music`
value is intentionally not a real path (systemd does NOT expand `%h` inside an
EnvironmentFile) — it forces the user to set an absolute path.

```sh
# Library roots and extra flags passed to exit66jukebox (-root is repeatable).
# Use absolute paths; %h is NOT expanded in this file.
EXIT66_ARGS=-root %h/Music   # edit me

# Optional scrobbling credentials (uncomment + fill):
#EXIT66_LISTENBRAINZ_TOKEN=
#EXIT66_LASTFM_API_KEY=
#EXIT66_LASTFM_API_SECRET=
```

- [ ] **Step 2: Verify content**

Run: `cat packaging/exit66.env.example`
Expected: the three EXIT66 sections above are present.

- [ ] **Step 3: Commit**

```bash
git add packaging/exit66.env.example
git commit -m "feat(packaging): add env file template"
```

---

### Task 3: `check-prereqs` Makefile target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add the target**

Append this target to `Makefile`. Each recipe line is a separate shell; the
check is written as a single `sh -c` block so the early `exit` works. Note the
doubled `$$` to pass literal `$` through Make to the shell.

```makefile
# Verify build prerequisites; print the distro-specific install command if missing.
check-prereqs:
	@missing=""; \
	command -v go >/dev/null 2>&1 || missing="$$missing go"; \
	command -v npm >/dev/null 2>&1 || missing="$$missing npm"; \
	if [ -n "$$missing" ]; then \
		echo "Missing build tools:$$missing"; \
		id=$$( . /etc/os-release 2>/dev/null; echo "$$ID" ); \
		case "$$id" in \
			arch) echo "Install with: sudo pacman -S go npm" ;; \
			fedora) echo "Install with: sudo dnf install golang npm" ;; \
			*) echo "Install go and npm with your distro's package manager." ;; \
		esac; \
		exit 1; \
	fi; \
	echo "prereqs OK"
```

- [ ] **Step 2: Add to `.PHONY`**

Change the first line of `Makefile` from:

```makefile
.PHONY: ui build test run clean
```

to:

```makefile
.PHONY: ui build test run clean check-prereqs install uninstall
```

- [ ] **Step 3: Verify it passes when tools are present**

Run: `make check-prereqs`
Expected: `prereqs OK` (go and npm are installed in this environment).

- [ ] **Step 4: Verify the failure path**

Run: `env PATH=/nonexistent make check-prereqs; echo exit=$?`
Expected: prints `Missing build tools: go npm`, an install hint line, and
`exit=2` (Make's exit code when the recipe exits 1).

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "feat(make): add check-prereqs target"
```

---

### Task 4: `install` and `uninstall` Makefile targets

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add path variables near the top of `Makefile`**

Add these after the `.PHONY` line. They centralize the install destinations.

```makefile
BIN_DIR  := $(HOME)/.local/bin
CONF_DIR := $(HOME)/.config/exit66jukebox
DATA_DIR := $(HOME)/.local/share/exit66jukebox
UNIT_DIR := $(HOME)/.config/systemd/user
```

- [ ] **Step 2: Add the `install` target**

Append to `Makefile`. The env file is installed only when absent so user edits
survive re-installs.

```makefile
# Build and install as a per-user systemd service (Arch/Fedora).
install: check-prereqs build
	@mkdir -p $(BIN_DIR) $(CONF_DIR) $(DATA_DIR) $(UNIT_DIR)
	install -m 0755 exit66jukebox $(BIN_DIR)/exit66jukebox
	install -m 0644 packaging/exit66jukebox.service $(UNIT_DIR)/exit66jukebox.service
	@if [ -f $(CONF_DIR)/exit66.env ]; then \
		echo "Keeping existing $(CONF_DIR)/exit66.env"; \
	else \
		install -m 0644 packaging/exit66.env.example $(CONF_DIR)/exit66.env; \
		echo "Installed env template to $(CONF_DIR)/exit66.env"; \
	fi
	systemctl --user daemon-reload
	@echo
	@echo "Next steps:"
	@echo "  1. Edit $(CONF_DIR)/exit66.env and set EXIT66_ARGS=-root /path/to/music"
	@echo "  2. systemctl --user enable --now exit66jukebox"
	@echo "  3. loginctl enable-linger $(USER)   # keep it running after logout"
```

- [ ] **Step 3: Add the `uninstall` target**

Append to `Makefile`. Leaves DB and env file in place.

```makefile
# Remove the binary and unit; keep DB and env file.
uninstall:
	rm -f $(BIN_DIR)/exit66jukebox $(UNIT_DIR)/exit66jukebox.service
	systemctl --user daemon-reload
	@echo "Removed binary and unit."
	@echo "Kept data dir  $(DATA_DIR)"
	@echo "Kept env file  $(CONF_DIR)/exit66.env"
```

- [ ] **Step 4: Run the install**

Run: `make install`
Expected: builds the UI + binary, prints "Installed env template…" (first run),
`daemon-reload` succeeds, and the three "Next steps" lines print.

- [ ] **Step 5: Verify the installed artifacts**

Run:
```bash
test -x ~/.local/bin/exit66jukebox && \
test -f ~/.config/systemd/user/exit66jukebox.service && \
test -f ~/.config/exit66jukebox/exit66.env && \
systemctl --user cat exit66jukebox >/dev/null && echo OK
```
Expected: `OK` (systemd can load the unit by name).

- [ ] **Step 6: Verify env file is not clobbered**

Run:
```bash
echo "# my edit" >> ~/.config/exit66jukebox/exit66.env
make install >/dev/null
grep -q "# my edit" ~/.config/exit66jukebox/exit66.env && echo "preserved"
```
Expected: `preserved`.

- [ ] **Step 7: Verify uninstall**

Run:
```bash
make uninstall
test ! -e ~/.local/bin/exit66jukebox && \
test ! -e ~/.config/systemd/user/exit66jukebox.service && \
test -f ~/.config/exit66jukebox/exit66.env && echo "OK"
```
Expected: `OK` (binary + unit gone, env file kept).

- [ ] **Step 8: Commit**

```bash
git add Makefile
git commit -m "feat(make): add install and uninstall targets"
```

---

### Task 5: README install section

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add an Install section**

Add this section to `README.md` after the `## Develop` section.

```markdown
## Install (Arch / Fedora)

Installs a per-user systemd service. Requires `go` and `npm`.

```sh
make install
# then edit the env file and enable the service:
$EDITOR ~/.config/exit66jukebox/exit66.env   # set EXIT66_ARGS=-root /path/to/music
systemctl --user enable --now exit66jukebox
loginctl enable-linger $USER                 # keep running after logout
```

The binary lands in `~/.local/bin`, config in `~/.config/exit66jukebox`, and
the SQLite DB in `~/.local/share/exit66jukebox`. `make uninstall` removes the
binary and unit, keeping your data and env file.
```

- [ ] **Step 2: Verify rendering**

Run: `sed -n '/## Install/,$p' README.md`
Expected: the Install section prints with the fenced `sh` block intact.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document make install"
```

---

## Self-Review Notes

- **Spec coverage:** unit (Task 1), env template (Task 2), `check-prereqs` with arch/fedora/other branches (Task 3), `install`/`uninstall` with no-clobber env + XDG dirs + daemon-reload + next-steps + linger tip (Task 4), README (Task 5). All spec sections covered.
- **No auto-enable / no sudo:** `install` only prints the enable + linger commands; it never runs them. Matches spec.
- **Path names consistent:** `BIN_DIR`/`CONF_DIR`/`DATA_DIR`/`UNIT_DIR` defined in Task 4 Step 1 and used unchanged in every later recipe line.
- **Make `$$` escaping:** Task 3 uses `$$` for shell variables and `. /etc/os-release` in a subshell so `$ID` resolves at runtime, not Make-expansion time.
