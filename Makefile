.PHONY: ui build test run clean check-prereqs install uninstall

BIN_DIR  := $(HOME)/.local/bin
CONF_DIR := $(HOME)/.config/exit66jukebox
DATA_DIR := $(HOME)/.local/share/exit66jukebox
UNIT_DIR := $(HOME)/.config/systemd/user

# Build the embedded web UI into internal/web/dist
ui:
	cd web && npm install && npm run build

# Build the single binary (rebuilds the UI first)
build: ui
	go build -o exit66jukebox .

test:
	go test ./...

run: build
	./exit66jukebox

clean:
	rm -f exit66jukebox

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

# Remove the binary and unit; keep DB and env file.
uninstall:
	rm -f $(BIN_DIR)/exit66jukebox $(UNIT_DIR)/exit66jukebox.service
	systemctl --user daemon-reload
	@echo "Removed binary and unit."
	@echo "Kept data dir  $(DATA_DIR)"
	@echo "Kept env file  $(CONF_DIR)/exit66.env"

# Verify build prerequisites; print the distro-specific install command if missing.
check-prereqs:
	@missing=""; \
	command -v go >/dev/null 2>&1 || missing="$$missing go"; \
	command -v npm >/dev/null 2>&1 || missing="$$missing npm"; \
	if [ -n "$$missing" ]; then \
		echo "Missing build tools:$$missing"; \
		id=$$( . /etc/os-release 2>/dev/null; echo "$$ID $$ID_LIKE" ); \
		case " $$id " in \
			*" arch "*) echo "Install with: sudo pacman -S go npm" ;; \
			*" fedora "*) echo "Install with: sudo dnf install golang npm" ;; \
			*) echo "Install go and npm with your distro's package manager." ;; \
		esac; \
		exit 1; \
	fi; \
	echo "prereqs OK"
