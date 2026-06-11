.PHONY: ui build test run clean

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
