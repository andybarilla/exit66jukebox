package api

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/sonos"
)

// streamURL builds the URL a Sonos will fetch for one stream, from a server IP
// and the server's listen address (for the port). It deliberately does NOT use
// the request Host header — that is client-controlled and could point the Sonos
// at an attacker's URL (Host injection).
func streamURL(ip, listenAddr, streamID string) string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		port = "8066"
	}
	return "http://" + net.JoinHostPort(ip, port) + streamPath(streamID)
}

// streamPath is the MP3 route for a stream id. One definition, because the cast
// URL is signed over exactly the path the speaker will then request.
func streamPath(streamID string) string { return "/stream/" + streamID + ".mp3" }

// castSignatureTTL is how long a cast URL's signature stays valid.
//
// It is deliberately long. A speaker holds no session cookie, so the signature
// in the URL is its only credential, and it re-fetches that same URL every time
// it reconnects — after a Wi-Fi blip, or when the household turns the speaker
// back on the next day. A cast already running survived the old six hours
// (one long connection, signed once when it opened), so the failure only ever
// showed up on a reconnect, as a 403 the speaker reports as silence (#130).
const castSignatureTTL = 30 * 24 * time.Hour

// castStreamURL returns a Sonos-reachable, signed URL for one stream. The signed
// token authorizes the speaker (which has no session cookie) to fetch that
// stream's own MP3 path. Callers must have established the stream is shared:
// a private stream never gets a broadcast pipeline, so this URL would 404.
func (s *Server) castStreamURL(streamID string) string {
	ip := sonos.OutboundIP()
	if ip == "" {
		ip = "127.0.0.1" // last resort; not Sonos-reachable, but never panics
	}
	base := streamURL(ip, s.listenAddr, streamID)
	exp := time.Now().Add(castSignatureTTL).Unix()
	return base + "?sig=" + auth.SignPath(s.signingSecret, streamPath(streamID), exp)
}

// streamIDFromURI recovers a stream id from a URI a speaker reports it is
// playing. It answers "" for anything that is not one of our MP3 stream paths —
// a Spotify session, a line-in, another player it is grouped with — so an
// unrecognised speaker reports no stream rather than a guess.
//
// Sonos is documented to rewrite a plain-http radio URI to
// x-rincon-mp3radio://<the original> on some paths, so that prefix is stripped
// before parsing. Every read from a real player during #130 echoed the URI back
// exactly as it was set, rewrite included nowhere — the strip is insurance, not
// something observed. The path shape is all this checks; the caller decides
// whether the id names a stream this server actually serves.
func streamIDFromURI(uri string) string {
	uri = strings.TrimPrefix(uri, "x-rincon-mp3radio://")
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	id, ok := strings.CutPrefix(u.Path, "/stream/")
	if !ok {
		return ""
	}
	id, ok = strings.CutSuffix(id, ".mp3")
	if !ok || id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

// deviceStream reports which stream a speaker is playing, or "" for a speaker
// that is stopped, unreachable, or playing anything else. Nothing is stored:
// this read off the device is the mapping, so a speaker somebody re-pointed
// with the Sonos app tells the truth here rather than contradicting us.
func (s *Server) deviceStream(ip string) string {
	uri, playing, err := s.deviceURI(ip)
	if err != nil || !playing {
		return ""
	}
	id := streamIDFromURI(uri)
	if id == "" || !s.isSharedStream(id) {
		return ""
	}
	return id
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

// castDevice is a speaker plus the stream it is currently playing. Stream is nil
// (JSON null) for a speaker playing nothing we recognise, which is also what a
// speaker that could not be reached reports.
type castDevice struct {
	sonos.Device
	Stream *string `json:"stream"`
}

// withStreams pairs each device with what it is playing, read off the devices
// themselves. The reads run concurrently because each one is a round trip to a
// speaker that may be asleep or gone, and a slow one must not add its timeout
// to every other device's. Each goroutine writes its own index of out and each
// closes over its own d — loop variables are per-iteration since Go 1.22 — so
// no lock is needed.
func (s *Server) withStreams(devices []sonos.Device) []castDevice {
	out := make([]castDevice, len(devices))
	var wg sync.WaitGroup
	for i, d := range devices {
		out[i] = castDevice{Device: d}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if id := s.deviceStream(d.IP); id != "" {
				out[i].Stream = &id
			}
		}()
	}
	wg.Wait()
	return out
}

// stopSpeakersPlaying stops every known speaker currently playing streamID, so a
// stream being deleted ends the cast cleanly instead of yanking the feed out
// from under the speaker. Known means the cast allowlist — the speakers already
// discovered or added by hand — rather than a fresh discovery, which would put
// an SSDP round trip in the middle of a delete.
//
// A speaker that cannot be reached is left alone: the stream is going away
// either way, and failing the delete over an unplugged speaker helps nobody.
func (s *Server) stopSpeakersPlaying(streamID string) {
	s.sonosMu.Lock()
	ips := make([]string, 0, len(s.sonosIPs)+len(s.sonosManual))
	for ip := range s.sonosIPs {
		ips = append(ips, ip)
	}
	for ip := range s.sonosManual {
		if !s.sonosIPs[ip] {
			ips = append(ips, ip)
		}
	}
	s.sonosMu.Unlock()

	// ip is per-iteration (Go 1.22+), so each goroutine stops its own speaker.
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.deviceStream(ip) != streamID {
				return
			}
			if err := s.stopCast(ip); err != nil {
				log.Printf("sonos: stopping %s before deleting stream %s: %v", ip, streamID, err)
			}
		}()
	}
	wg.Wait()
}

func (s *Server) sonosDevices(w http.ResponseWriter, r *http.Request) {
	// SSDP multicast is unreliable on some LANs (see #62). When it finds nothing —
	// whether it returned cleanly empty or errored — fall back to a unicast /24
	// scan rather than failing the request.
	devices, _ := s.sonosDiscover()
	if len(devices) == 0 {
		devices = s.scanUnicast()
	}
	if devices == nil {
		devices = []sonos.Device{}
	}
	s.rememberDevices(devices)
	writeJSON(w, http.StatusOK, s.withStreams(s.deviceList(devices)))
}

// castStreamID reads the stream to cast from the request. An absent stream is
// the house stream, so a client that predates per-stream casting keeps working.
//
// Only a shared stream can be cast, and that single check covers every refusal
// the stream routes make: an unknown id, a private row, and the personal-stream
// alias all fail it. It answers 404 the way streamGate does, rather than
// revealing by contrast which ids exist.
func (s *Server) castStreamID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.FormValue("stream")
	if id == "" {
		id = houseStreamID
	}
	if !s.isSharedStream(id) {
		writeErr(w, http.StatusNotFound, "no such shared stream")
		return "", false
	}
	return id, true
}

func (s *Server) sonosCast(w http.ResponseWriter, r *http.Request) {
	ip, ok := s.castTarget(w, r)
	if !ok {
		return
	}
	streamID, ok := s.castStreamID(w, r)
	if !ok {
		return
	}
	if err := s.castTo(ip, s.castStreamURL(streamID), "Exit 66 Jukebox"); err != nil {
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
	if err := s.stopCast(ip); err != nil {
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
