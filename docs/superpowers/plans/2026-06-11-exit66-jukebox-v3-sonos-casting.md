# Exit 66 Jukebox — Plan 3: Sonos Casting

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Discover Sonos players on the LAN and cast the always-on house stream to a chosen player (and stop it), from the web UI.

**Architecture:** A new `internal/sonos` package does two things: (1) **SSDP discovery** — multicast `M-SEARCH` for `ZonePlayer` devices, parse each responder's descriptor for its room name; (2) **AVTransport control** — SOAP `SetAVTransportURI` + `Play` to point a Sonos at `http://<server-lan-ip>:<port>/stream/house.mp3`, and `Stop`. The API layer exposes `GET /api/sonos/devices`, `POST /api/sonos/cast`, `POST /api/sonos/stop`. The cast URL is derived from the request's `Host` header (the LAN address the browser used to reach us — which the Sonos can also reach), falling back to the server's detected outbound IP if the request came via loopback. The UI adds a "Cast to Sonos" picker with a Stop control. The house stream is already Sonos-ready (`Content-Type: audio/mpeg`, continuous, real-time paced), so no streaming changes are needed.

**Tech Stack:** Go stdlib only (`net` UDP multicast, `net/http`, `encoding/xml`). No new deps. Builds on Plan 2 (the house stream).

**Module path:** `github.com/andybarilla/exit66jukebox`. **Branch:** `plan3-sonos` off `plan2-shared-stream`.

## Design decisions (settled)
- **Scope: cast + stop.** No volume or play/pause in v1 (play/pause on a live radio feed is awkward).
- **Discovery: SSDP auto-discovery**, no config. Sonos control lives at the fixed `http://<ip>:1400/MediaRenderer/AVTransport/Control`, so we only need each player's IP + room name.
- **Reachable URL:** prefer `r.Host` (the LAN address the browser used); if loopback, substitute the detected outbound LAN IP with the configured port. This avoids a config flag in the common case.
- **Testable seams:** SSDP and real SOAP-to-Sonos can't be unit-tested without hardware, so the *pure* parts (SSDP `LOCATION` parsing, descriptor-name parsing, host extraction) and the *SOAP envelope/transport* (against an `httptest.Server` mock Sonos) are unit-tested; live discovery + a real cast are a manual smoke test.

## File Structure
| Path | Responsibility |
|------|----------------|
| `internal/sonos/discover.go` | SSDP M-SEARCH, response/descriptor parsing, `Discover(timeout)` |
| `internal/sonos/control.go` | `Cast(ip,url,title)`, `Stop(ip)`, SOAP envelope, DIDL metadata, `OutboundIP()` |
| `internal/api/sonos.go` | `GET /api/sonos/devices`, `POST /api/sonos/cast`, `POST /api/sonos/stop`; cast-URL derivation |
| `internal/api/server.go` (modify) | register the three routes |
| `web/src/lib/api.js` (modify) | `listSonos()`, `castSonos(ip)`, `stopSonos(ip)` |
| `web/src/App.svelte` (modify) | "Cast to Sonos" picker + Stop |

---

## Phase 1 — SSDP discovery

### Task 1.1: Parse SSDP responses and device descriptors (pure functions)

**Files:** Create `internal/sonos/discover.go`, `internal/sonos/discover_test.go`.

- [ ] **Step 1: Write the failing test**

`internal/sonos/discover_test.go`:
```go
package sonos

import "testing"

const sampleSSDP = "HTTP/1.1 200 OK\r\n" +
	"CACHE-CONTROL: max-age = 1800\r\n" +
	"LOCATION: http://192.168.1.50:1400/xml/device_description.xml\r\n" +
	"ST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n\r\n"

const sampleDesc = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <friendlyName>192.168.1.50 - Sonos Play:1</friendlyName>
    <roomName>Living Room</roomName>
  </device>
</root>`

func TestParseLocation(t *testing.T) {
	got := parseLocation(sampleSSDP)
	want := "http://192.168.1.50:1400/xml/device_description.xml"
	if got != want {
		t.Fatalf("parseLocation = %q, want %q", got, want)
	}
	if parseLocation("HTTP/1.1 200 OK\r\nST: x\r\n\r\n") != "" {
		t.Fatalf("expected empty location when header absent")
	}
}

func TestHostOf(t *testing.T) {
	if got := hostOf("http://192.168.1.50:1400/xml/x.xml"); got != "192.168.1.50" {
		t.Fatalf("hostOf = %q", got)
	}
}

func TestParseDeviceNamePrefersRoomName(t *testing.T) {
	if got := parseDeviceName([]byte(sampleDesc)); got != "Living Room" {
		t.Fatalf("parseDeviceName = %q, want Living Room", got)
	}
	if got := parseDeviceName([]byte("not xml")); got != "" {
		t.Fatalf("expected empty name on bad xml, got %q", got)
	}
}
```
Run `mise exec -- go test ./internal/sonos/` → FAIL (undefined).

- [ ] **Step 2: Implement `internal/sonos/discover.go`**
```go
package sonos

import (
	"bufio"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Device is a discovered Sonos player.
type Device struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

const (
	ssdpAddr     = "239.255.255.250:1900"
	searchTarget = "urn:schemas-upnp-org:device:ZonePlayer:1"
)

// Discover sends an SSDP M-SEARCH and returns the Sonos ZonePlayers that respond
// within timeout, deduped by IP.
func Discover(timeout time.Duration) ([]Device, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}
	msg := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: " + ssdpAddr + "\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 1\r\n" +
		"ST: " + searchTarget + "\r\n\r\n"
	if _, err := conn.WriteTo([]byte(msg), dst); err != nil {
		return nil, err
	}

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	seen := map[string]bool{}
	var devices []Device
	buf := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break // deadline reached
		}
		loc := parseLocation(string(buf[:n]))
		ip := hostOf(loc)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		name := fetchName(loc)
		if name == "" {
			name = ip
		}
		devices = append(devices, Device{Name: name, IP: ip})
	}
	return devices, nil
}

// parseLocation extracts the LOCATION header from an SSDP response.
func parseLocation(resp string) string {
	sc := bufio.NewScanner(strings.NewReader(resp))
	for sc.Scan() {
		line := sc.Text()
		if len(line) >= 9 && strings.EqualFold(line[:9], "LOCATION:") {
			return strings.TrimSpace(line[9:])
		}
	}
	return ""
}

// hostOf returns the host of an http URL like http://1.2.3.4:1400/...
func hostOf(location string) string {
	u := strings.TrimPrefix(location, "http://")
	if i := strings.IndexAny(u, ":/"); i >= 0 {
		return u[:i]
	}
	return u
}

type deviceDesc struct {
	Device struct {
		RoomName     string `xml:"roomName"`
		FriendlyName string `xml:"friendlyName"`
	} `xml:"device"`
}

// parseDeviceName extracts a human name (room name preferred) from a Sonos
// device descriptor XML.
func parseDeviceName(b []byte) string {
	var d deviceDesc
	if err := xml.Unmarshal(b, &d); err != nil {
		return ""
	}
	if d.Device.RoomName != "" {
		return d.Device.RoomName
	}
	return d.Device.FriendlyName
}

func fetchName(location string) string {
	if location == "" {
		return ""
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(location)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return parseDeviceName(b)
}
```
Run `mise exec -- go test ./internal/sonos/` → PASS.

- [ ] **Step 3: Commit**
```bash
git add internal/sonos/discover.go internal/sonos/discover_test.go
git commit -m "feat(sonos): SSDP discovery with response/descriptor parsing"
```

---

## Phase 2 — AVTransport control (cast + stop)

### Task 2.1: SOAP Cast/Stop against a mock Sonos

**Files:** Create `internal/sonos/control.go`, `internal/sonos/control_test.go`.

- [ ] **Step 1: Write the failing test** (mock Sonos via httptest)

`internal/sonos/control_test.go`:
```go
package sonos

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCastURLSendsSetAndPlay(t *testing.T) {
	var actions []string
	var lastBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actions = append(actions, r.Header.Get("SOAPACTION"))
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := castURL(srv.URL, "http://10.0.0.2:8066/stream/house.mp3", "Exit 66"); err != nil {
		t.Fatalf("castURL: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 SOAP calls (SetAVTransportURI, Play), got %d", len(actions))
	}
	if !strings.Contains(actions[0], "SetAVTransportURI") {
		t.Fatalf("first action should be SetAVTransportURI, got %q", actions[0])
	}
	if !strings.Contains(actions[1], "Play") {
		t.Fatalf("second action should be Play, got %q", actions[1])
	}
	if !strings.Contains(lastBody, "Play") {
		t.Fatalf("last body should be the Play envelope")
	}
}

func TestCastURLSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if err := castURL(srv.URL, "http://x/stream/house.mp3", "t"); err == nil {
		t.Fatalf("expected error on 500 response")
	}
}

func TestStopURLSendsStop(t *testing.T) {
	var action string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action = r.Header.Get("SOAPACTION")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := stopURL(srv.URL); err != nil {
		t.Fatalf("stopURL: %v", err)
	}
	if !strings.Contains(action, "Stop") {
		t.Fatalf("expected Stop action, got %q", action)
	}
}
```
Run `mise exec -- go test ./internal/sonos/ -run 'Cast|Stop'` → FAIL (undefined).

- [ ] **Step 2: Implement `internal/sonos/control.go`**
```go
package sonos

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const avTransport = "urn:schemas-upnp-org:service:AVTransport:1"

// ControlURL is the fixed AVTransport control endpoint for a Sonos player.
func ControlURL(ip string) string {
	return fmt.Sprintf("http://%s:1400/MediaRenderer/AVTransport/Control", ip)
}

// Cast points the Sonos at streamURL and starts playback.
func Cast(ip, streamURL, title string) error { return castURL(ControlURL(ip), streamURL, title) }

// Stop stops playback on the Sonos.
func Stop(ip string) error { return stopURL(ControlURL(ip)) }

func castURL(controlURL, streamURL, title string) error {
	set := fmt.Sprintf(
		`<u:SetAVTransportURI xmlns:u="%s"><InstanceID>0</InstanceID>`+
			`<CurrentURI>%s</CurrentURI><CurrentURIMetaData>%s</CurrentURIMetaData>`+
			`</u:SetAVTransportURI>`,
		avTransport, xmlEscape(streamURL), xmlEscape(didl(title)))
	if err := soap(controlURL, "SetAVTransportURI", set); err != nil {
		return err
	}
	play := fmt.Sprintf(`<u:Play xmlns:u="%s"><InstanceID>0</InstanceID><Speed>1</Speed></u:Play>`, avTransport)
	return soap(controlURL, "Play", play)
}

func stopURL(controlURL string) error {
	body := fmt.Sprintf(`<u:Stop xmlns:u="%s"><InstanceID>0</InstanceID></u:Stop>`, avTransport)
	return soap(controlURL, "Stop", body)
}

func soap(controlURL, action, inner string) error {
	env := `<?xml version="1.0"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" ` +
		`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		inner + `</s:Body></s:Envelope>`
	req, err := http.NewRequest(http.MethodPost, controlURL, strings.NewReader(env))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"`+avTransport+`#`+action+`"`)
	resp, err := (&http.Client{Timeout: 4 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sonos %s: status %d", action, resp.StatusCode)
	}
	return nil
}

func didl(title string) string {
	return `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" ` +
		`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` +
		`<item id="0" parentID="-1" restricted="1"><dc:title>` + xmlEscape(title) +
		`</dc:title><upnp:class>object.item.audioItem.audioBroadcast</upnp:class>` +
		`</item></DIDL-Lite>`
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// OutboundIP returns the preferred outbound LAN IP of this host (no packets are
// actually sent — UDP "connect" just selects the route). Empty on failure.
func OutboundIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
```
Run `mise exec -- go test ./internal/sonos/` → PASS.

- [ ] **Step 3: Commit**
```bash
git add internal/sonos/control.go internal/sonos/control_test.go
git commit -m "feat(sonos): AVTransport cast/stop SOAP control + outbound IP"
```

---

## Phase 3 — API endpoints

### Task 3.1: devices / cast / stop handlers

**Files:** Create `internal/api/sonos.go`, `internal/api/sonos_test.go`; modify `internal/api/server.go`.

- [ ] **Step 1: Register routes** in `internal/api/server.go` `Handler()` (before `if s.ui != nil`):
```go
	mux.HandleFunc("GET /api/sonos/devices", s.sonosDevices)
	mux.HandleFunc("POST /api/sonos/cast", s.sonosCast)
	mux.HandleFunc("POST /api/sonos/stop", s.sonosStop)
```

- [ ] **Step 2: Write the failing test**

`internal/api/sonos_test.go`:
```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSonosCastRequiresIP(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/sonos/cast",
		strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 when ip missing, got %d", rec.Code)
	}
}

func TestHouseStreamURLFromHost(t *testing.T) {
	got := houseStreamURL("192.168.1.10:8066")
	want := "http://192.168.1.10:8066/stream/house.mp3"
	if got != want {
		t.Fatalf("houseStreamURL = %q, want %q", got, want)
	}
}
```
Run `mise exec -- go test ./internal/api/ -run 'Sonos|HouseStream'` → FAIL.

- [ ] **Step 3: Implement `internal/api/sonos.go`**
```go
package api

import (
	"net"
	"net/http"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/sonos"
)

// houseStreamURL builds a LAN-reachable URL for the house stream from the host
// the browser used to reach us. If host is loopback, substitute the server's
// detected outbound IP (keeping the port) so the Sonos can reach it.
func houseStreamURL(host string) string {
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		h = host
		port = ""
	}
	if h == "127.0.0.1" || h == "localhost" || h == "::1" {
		if ip := sonos.OutboundIP(); ip != "" {
			h = ip
		}
	}
	if port != "" {
		h = net.JoinHostPort(h, port)
	}
	return "http://" + h + "/stream/house.mp3"
}

func (s *Server) sonosDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := sonos.Discover(2 * time.Second)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if devices == nil {
		devices = []sonos.Device{}
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) sonosCast(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ip := r.FormValue("ip")
	if ip == "" {
		writeErr(w, http.StatusBadRequest, "missing ip")
		return
	}
	if err := sonos.Cast(ip, houseStreamURL(r.Host), "Exit 66 Jukebox"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) sonosStop(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ip := r.FormValue("ip")
	if ip == "" {
		writeErr(w, http.StatusBadRequest, "missing ip")
		return
	}
	if err := sonos.Stop(ip); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
```
Run `mise exec -- go test ./internal/api/ -run 'Sonos|HouseStream'` → PASS. Run `mise exec -- go test -race ./...` → all pass.

- [ ] **Step 4: Commit**
```bash
git add internal/api/sonos.go internal/api/sonos_test.go internal/api/server.go
git commit -m "feat(api): sonos devices/cast/stop endpoints"
```

---

## Phase 4 — UI: Cast to Sonos

### Task 4.1: API client + cast picker

**Files:** modify `web/src/lib/api.js`, `web/src/App.svelte`.

- [ ] **Step 1: Append to `web/src/lib/api.js`**
```js
export async function listSonos() {
  const r = await fetch('/api/sonos/devices');
  return r.json(); // [{name, ip}]
}
export async function castSonos(ip) {
  const r = await fetch('/api/sonos/cast', { method: 'POST', body: new URLSearchParams({ ip }) });
  return r.json();
}
export async function stopSonos(ip) {
  const r = await fetch('/api/sonos/stop', { method: 'POST', body: new URLSearchParams({ ip }) });
  return r.json();
}
```

- [ ] **Step 2: Add a Cast control to `web/src/App.svelte`**

In the `<script>`, add imports `listSonos, castSonos, stopSonos` and state:
```js
  let sonosDevices = [];
  let castIP = null;
  let sonosBusy = false;

  async function loadSonos() {
    sonosBusy = true;
    sonosDevices = await listSonos();
    sonosBusy = false;
  }
  async function cast(ip) { await castSonos(ip); castIP = ip; }
  async function stopCast() { if (castIP) { await stopSonos(castIP); castIP = null; } }
```

In the markup, add (e.g. below the mode toggle):
```svelte
  <div class="sonos">
    <button on:click={loadSonos} disabled={sonosBusy}>
      {sonosBusy ? 'Searching…' : 'Cast to Sonos'}
    </button>
    {#each sonosDevices as d (d.ip)}
      <button class:active={castIP === d.ip} on:click={() => cast(d.ip)}>{d.name}</button>
    {/each}
    {#if castIP}
      <button on:click={stopCast}>Stop casting</button>
    {/if}
  </div>
```
Add a `.sonos button.active { background: #6cf; color: #000; }` style (mirror `.modes`).

- [ ] **Step 3: Rebuild + verify**
```bash
cd web && mise exec -- npm run build && cd ..
mise exec -- go build ./... && mise exec -- go test ./...
```
Both clean / all pass.

- [ ] **Step 4: Manual smoke test (with a real Sonos on the LAN)**
```bash
mise exec -- go build -o exit66jukebox .
./exit66jukebox -root /path/to/music -db /tmp/e66.db
```
From a browser on the LAN: queue a track into House, click **Cast to Sonos**, confirm the room appears, click it, confirm the Sonos starts playing the house stream; click **Stop casting**, confirm it stops. Also verify `curl -s localhost:8066/api/sonos/devices` returns the player list.

- [ ] **Step 5: Commit (source + rebuilt dist)**
```bash
git add web/src/ internal/web/dist
git commit -m "feat(ui): cast the house stream to a Sonos player"
```

---

## Self-Review notes (addressed)
- **Coverage:** discovery (Phase 1), cast+stop control (Phase 2), API (Phase 3), UI (Phase 4). Cast+stop only (volume/transport deferred). SSDP auto-discovery (no manual entry — a future addition if multicast is blocked).
- **Testable seams:** pure parsers (`parseLocation`, `hostOf`, `parseDeviceName`) and SOAP transport (mock httptest Sonos) are unit-tested; live SSDP + a real cast are the Phase 4 manual smoke test (cannot be unit-tested without hardware).
- **Reachable URL:** derived from `r.Host`; loopback falls back to `OutboundIP()`. No config flag needed in the common LAN case.
- **No streaming changes:** the house stream is already `audio/mpeg`, continuous, real-time — Sonos-ready as shipped in Plan 2.

## Definition of done
`go test ./...` passes; from a LAN browser, **Cast to Sonos** lists the room(s), casting starts the house stream on the chosen Sonos, and **Stop casting** stops it. Volume/transport control and manual-IP fallback are explicitly out of scope.
