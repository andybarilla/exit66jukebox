package api

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/sonos"
)

// streamURL builds the house-stream URL the Sonos will fetch, from a server IP
// and the server's listen address (for the port). It deliberately does NOT use
// the request Host header — that is client-controlled and could point the Sonos
// at an attacker's URL (Host injection).
func streamURL(ip, listenAddr string) string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		port = "8066"
	}
	return "http://" + net.JoinHostPort(ip, port) + "/stream/house.mp3"
}

// houseStreamURL returns a Sonos-reachable URL for the house stream using the
// server's detected outbound LAN IP and configured port.
func (s *Server) houseStreamURL() string {
	ip := sonos.OutboundIP()
	if ip == "" {
		ip = "127.0.0.1" // last resort; not Sonos-reachable, but never panics
	}
	return streamURL(ip, s.listenAddr)
}

// rememberDevices records the discovered device IPs as the cast allowlist.
func (s *Server) rememberDevices(devices []sonos.Device) {
	ips := make(map[string]bool, len(devices))
	for _, d := range devices {
		ips[d.IP] = true
	}
	s.sonosMu.Lock()
	s.sonosIPs = ips
	s.sonosMu.Unlock()
}

// allowedSonos reports whether ip was seen in the most recent discovery or was
// added manually. Manual IPs live in a separate set so rediscovery can't wipe
// them.
func (s *Server) allowedSonos(ip string) bool {
	s.sonosMu.Lock()
	defer s.sonosMu.Unlock()
	_, manual := s.sonosManual[ip]
	return s.sonosIPs[ip] || manual
}

// deviceList merges the freshly discovered devices with the manually-added ones
// so manual IPs stay visible and castable in the UI.
func (s *Server) deviceList(discovered []sonos.Device) []sonos.Device {
	s.sonosMu.Lock()
	defer s.sonosMu.Unlock()
	list := make([]sonos.Device, 0, len(discovered)+len(s.sonosManual))
	seen := make(map[string]bool, len(discovered))
	for _, d := range discovered {
		list = append(list, d)
		seen[d.IP] = true
	}
	for ip, name := range s.sonosManual {
		if !seen[ip] {
			list = append(list, sonos.Device{Name: name, IP: ip})
		}
	}
	return list
}

// privateIPv4 rejects anything that isn't a routable private LAN IPv4 — blocks
// loopback, link-local (incl. 169.254 metadata), multicast, and public IPs.
func privateIPv4(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	v4 := p.To4()
	if v4 == nil || p.IsLoopback() || p.IsLinkLocalUnicast() || p.IsMulticast() {
		return false
	}
	switch {
	case v4[0] == 10:
		return true
	case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
		return true
	case v4[0] == 192 && v4[1] == 168:
		return true
	}
	return false
}

// castTarget validates the requested ip: it must be a private IPv4 AND have been
// returned by a recent discovery. This closes the SSRF surface — the server will
// only POST control requests to IPs that actually announced themselves as Sonos
// players on the LAN, never to an arbitrary host supplied by the caller.
func (s *Server) castTarget(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.ParseForm()
	ip := r.FormValue("ip")
	if !privateIPv4(ip) || !s.allowedSonos(ip) {
		writeErr(w, http.StatusBadRequest, "unknown or invalid device")
		return "", false
	}
	return ip, true
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
	s.rememberDevices(devices)
	writeJSON(w, http.StatusOK, s.deviceList(devices))
}

func (s *Server) sonosCast(w http.ResponseWriter, r *http.Request) {
	ip, ok := s.castTarget(w, r)
	if !ok {
		return
	}
	if err := sonos.Cast(ip, s.houseStreamURL(), "Exit 66 Jukebox"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) sonosStop(w http.ResponseWriter, r *http.Request) {
	ip, ok := s.castTarget(w, r)
	if !ok {
		return
	}
	if err := sonos.Stop(ip); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) sonosGetVolume(w http.ResponseWriter, r *http.Request) {
	ip, ok := s.castTarget(w, r)
	if !ok {
		return
	}
	vol, err := sonos.GetVolume(ip)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"volume": vol})
}

func (s *Server) sonosSetVolume(w http.ResponseWriter, r *http.Request) {
	ip, ok := s.castTarget(w, r)
	if !ok {
		return
	}
	vol, _ := strconv.Atoi(r.FormValue("volume"))
	if vol < 0 {
		vol = 0
	} else if vol > 100 {
		vol = 100
	}
	if err := sonos.SetVolume(ip, vol); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// sonosManualAdd trusts a manually-entered IP only after confirming it's a
// private LAN IPv4 AND actually serves a Sonos device descriptor — the same
// two-part SSRF guard discovery relies on, for SSDP-blocked networks.
func (s *Server) sonosManualAdd(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	ip := r.FormValue("ip")
	if !privateIPv4(ip) {
		writeErr(w, http.StatusBadRequest, "invalid ip")
		return
	}
	name, ok := s.manualVerify(ip)
	if !ok {
		writeErr(w, http.StatusBadGateway, "not a Sonos device")
		return
	}
	s.sonosMu.Lock()
	s.sonosManual[ip] = name
	s.sonosMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "ip": ip})
}
