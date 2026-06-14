.PHONY: ui build test run clean check-prereqs install uninstall

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
