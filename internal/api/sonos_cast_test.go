package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/auth"
	"github.com/andybarilla/exit66jukebox/internal/sonos"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

// castPost casts to ip with an optional stream id, as an admin.
func castPost(t *testing.T, srv *Server, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sonos/cast", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// castCapture points the server's Sonos calls at a recorder instead of the LAN,
// and puts ip on the discovery allowlist so castTarget accepts it.
func castCapture(srv *Server, ips ...string) *map[string]string {
	cast := map[string]string{}
	devs := make([]sonos.Device, 0, len(ips))
	for _, ip := range ips {
		devs = append(devs, sonos.Device{Name: "Speaker " + ip, IP: ip})
	}
	srv.rememberDevices(devs)
	srv.castTo = func(ip, url, title string) error { cast[ip] = url; return nil }
	return &cast
}

func TestCastAcceptsASharedStreamOtherThanHouse(t *testing.T) {
	srv, db := newTestServer(t)
	cookie := adminSession(t, db)
	if err := store.CreateSharedStream(db, "party01", "Party"); err != nil {
		t.Fatalf("create: %v", err)
	}
	cast := castCapture(srv, "192.168.1.50")

	if rec := castPost(t, srv, "ip=192.168.1.50&stream=party01", cookie); rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	got := (*cast)["192.168.1.50"]
	if !strings.Contains(got, "/stream/party01.mp3") {
		t.Fatalf("cast URL = %q, want the party stream's own path", got)
	}
}

// Two speakers, two streams, at the same time: nothing on the server ties a
// device to a stream, so the second cast must not disturb the first.
func TestCastTwoSpeakersTwoStreams(t *testing.T) {
	srv, db := newTestServer(t)
	cookie := adminSession(t, db)
	store.CreateSharedStream(db, "party01", "Party")
	cast := castCapture(srv, "192.168.1.50", "192.168.1.51")

	if rec := castPost(t, srv, "ip=192.168.1.50&stream=house", cookie); rec.Code != http.StatusOK {
		t.Fatalf("house cast: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := castPost(t, srv, "ip=192.168.1.51&stream=party01", cookie); rec.Code != http.StatusOK {
		t.Fatalf("party cast: %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains((*cast)["192.168.1.50"], "/stream/house.mp3") {
		t.Fatalf("first speaker = %q, want house", (*cast)["192.168.1.50"])
	}
	if !strings.Contains((*cast)["192.168.1.51"], "/stream/party01.mp3") {
		t.Fatalf("second speaker = %q, want party01", (*cast)["192.168.1.51"])
	}
}

// No stream parameter is the house stream: every client that predates per-stream
// casting keeps working unchanged.
func TestCastWithoutStreamIsHouse(t *testing.T) {
	srv, db := newTestServer(t)
	cast := castCapture(srv, "192.168.1.50")
	if rec := castPost(t, srv, "ip=192.168.1.50", adminSession(t, db)); rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains((*cast)["192.168.1.50"], "/stream/house.mp3") {
		t.Fatalf("cast URL = %q, want the house stream", (*cast)["192.168.1.50"])
	}
}

// A private stream has no broadcast pipeline, so it can never be served to a
// speaker. The refusal must happen before any SOAP call leaves the server.
func TestCastRefusesPrivateAndUnknownStreams(t *testing.T) {
	srv, db := newTestServer(t)
	cookie := adminSession(t, db)
	if err := store.EnsurePrivateStream(db, store.PersonalStreamID(1)); err != nil {
		t.Fatalf("ensure private: %v", err)
	}
	cast := castCapture(srv, "192.168.1.50")

	for _, id := range []string{
		store.PersonalStreamID(1), // somebody's personal stream, named directly
		store.PersonalStreamAlias, // the alias the client uses for "mine"
		"nosuchstream",            // never existed
	} {
		rec := castPost(t, srv, "ip=192.168.1.50&stream="+id, cookie)
		if rec.Code != http.StatusNotFound {
			t.Errorf("stream=%q: want 404, got %d (%s)", id, rec.Code, rec.Body.String())
		}
	}
	if len(*cast) != 0 {
		t.Fatalf("a refused cast must not reach the speaker, got %v", *cast)
	}
}

// The device allowlist still has to hold with a stream id in play: naming a
// valid stream must not buy a cast to an arbitrary internal host.
func TestCastStreamDoesNotBypassDeviceAllowlist(t *testing.T) {
	srv, db := newTestServer(t)
	cast := castCapture(srv, "192.168.1.50")
	rec := castPost(t, srv, "ip=192.168.1.99&stream=house", adminSession(t, db))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for an undiscovered ip, got %d", rec.Code)
	}
	if len(*cast) != 0 {
		t.Fatalf("must not have cast to an unknown device, got %v", *cast)
	}
}

// The cast URL's signature must outlive a speaker's reconnect. Six hours meant a
// speaker picked up the next day fetched the same URL and got a bare 403.
func TestCastURLSignatureOutlivesTheDay(t *testing.T) {
	srv, _ := newTestServer(t)
	raw := srv.castStreamURL("house")
	i := strings.Index(raw, "?sig=")
	if i < 0 {
		t.Fatalf("cast URL has no signature: %q", raw)
	}
	sig := raw[i+len("?sig="):]

	now := time.Now()
	for _, d := range []time.Duration{time.Hour, 7 * 24 * time.Hour, 29 * 24 * time.Hour} {
		if !auth.VerifyPath(srv.signingSecret, sig, "/stream/house.mp3", now.Add(d).Unix()) {
			t.Errorf("signature rejected %v after minting; a speaker reconnecting then gets a silent 403", d)
		}
	}
	if auth.VerifyPath(srv.signingSecret, sig, "/stream/house.mp3", now.Add(31*24*time.Hour).Unix()) {
		t.Errorf("signature still valid after 31 days; it is meant to expire eventually")
	}
}

func TestStreamIDFromURI(t *testing.T) {
	for uri, want := range map[string]string{
		"http://192.168.1.10:8066/stream/house.mp3?sig=1.abc":         "house",
		"http://192.168.1.10:8066/stream/a1b2c3d4.mp3":                "a1b2c3d4",
		"x-rincon-mp3radio://http://10.0.0.2:8066/stream/party01.mp3": "party01", // Sonos's own radio rewrite
		"x-sonos-vli:RINCON_C4387520F3EC01400:2,spotify:0ef0434c":     "",
		"x-rincon:RINCON_38420B6DD01601400":                           "", // grouped with another player
		"http://192.168.1.10:8066/stream/.mp3":                        "",
		"http://192.168.1.10:8066/stream/house":                       "", // no .mp3 suffix
		"http://192.168.1.10:8066/other/house.mp3":                    "",
		"http://radio.example.com/stream/house.mp3":                   "house", // path shape only; the caller checks the id is ours
		"":            "",
		"::not a uri": "",
	} {
		if got := streamIDFromURI(uri); got != want {
			t.Errorf("streamIDFromURI(%q) = %q, want %q", uri, got, want)
		}
	}
}

func castDevicesJSON(t *testing.T, srv *Server) []map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/sonos/devices", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET devices: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	return list
}

// The device list is the whole device-to-stream mapping, and it is read off the
// speakers rather than stored: a device playing a stream reports its id, one
// playing anything else reports nothing.
func TestDeviceListReportsWhatEachSpeakerIsPlaying(t *testing.T) {
	srv, db := newTestServer(t)
	store.CreateSharedStream(db, "party01", "Party")
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.sonosDiscover = func() ([]sonos.Device, error) {
		return []sonos.Device{
			{Name: "Kitchen", IP: "192.168.1.50"},
			{Name: "Patio", IP: "192.168.1.51"},
			{Name: "Den", IP: "192.168.1.52"},
			{Name: "Study", IP: "192.168.1.53"},
			{Name: "Attic", IP: "192.168.1.54"},
		}, nil
	}
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.deviceURI = func(ip string) (string, bool, error) {
		switch ip {
		case "192.168.1.50":
			return "http://192.168.1.10:8066/stream/house.mp3?sig=1.a", true, nil
		case "192.168.1.51":
			return "http://192.168.1.10:8066/stream/party01.mp3?sig=1.a", true, nil
		case "192.168.1.52":
			return "x-sonos-vli:RINCON_C438:2,spotify:0ef0", true, nil // playing Spotify
		case "192.168.1.53":
			// Stopped, but still reporting the URI it was last pointed at.
			return "http://192.168.1.10:8066/stream/house.mp3?sig=1.a", false, nil
		}
		return "", false, nil
	}

	got := map[string]any{}
	for _, d := range castDevicesJSON(t, srv) {
		got[d["ip"].(string)] = d["stream"]
	}
	want := map[string]any{
		"192.168.1.50": "house",
		"192.168.1.51": "party01",
		"192.168.1.52": nil,
		"192.168.1.53": nil,
		"192.168.1.54": nil,
	}
	for ip, w := range want {
		if got[ip] != w {
			t.Errorf("device %s stream = %v, want %v", ip, got[ip], w)
		}
	}
}

// A URL whose path parses but names a stream this server does not serve is not
// a guess worth reporting.
func TestDeviceListIgnoresAnUnknownStreamID(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.sonosDiscover = func() ([]sonos.Device, error) {
		return []sonos.Device{{Name: "Kitchen", IP: "192.168.1.50"}}, nil
	}
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.deviceURI = func(string) (string, bool, error) {
		return "http://elsewhere.example/stream/deleted99.mp3", true, nil
	}
	list := castDevicesJSON(t, srv)
	if len(list) != 1 || list[0]["stream"] != nil {
		t.Fatalf("stream = %v, want nil for an id this server does not serve", list[0]["stream"])
	}
}

// A speaker that cannot be reached must not fail the whole device list — the
// other speakers are still worth listing.
func TestDeviceListSurvivesAnUnreachableSpeaker(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.sonosDiscover = func() ([]sonos.Device, error) {
		return []sonos.Device{
			{Name: "Kitchen", IP: "192.168.1.50"},
			{Name: "Patio", IP: "192.168.1.51"},
		}, nil
	}
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.deviceURI = func(ip string) (string, bool, error) {
		if ip == "192.168.1.50" {
			return "", false, http.ErrHandlerTimeout
		}
		return "http://192.168.1.10:8066/stream/house.mp3", true, nil
	}
	got := map[string]any{}
	for _, d := range castDevicesJSON(t, srv) {
		got[d["ip"].(string)] = d["stream"]
	}
	if len(got) != 2 || got["192.168.1.50"] != nil || got["192.168.1.51"] != "house" {
		t.Fatalf("devices = %v, want the reachable speaker still reported", got)
	}
}

// Deleting a stream a speaker is playing must stop that speaker first, so the
// speaker ends the cast itself instead of having the feed vanish under it.
func TestDeleteStreamStopsSpeakersPlayingIt(t *testing.T) {
	srv, db := newTestServer(t)
	cookie := adminSession(t, db)
	store.CreateSharedStream(db, "party01", "Party")
	srv.rememberDevices([]sonos.Device{
		{Name: "Kitchen", IP: "192.168.1.50"},
		{Name: "Patio", IP: "192.168.1.51"},
	})
	srv.sonosManual["192.168.1.52"] = "Den" // manual speakers count too
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.deviceURI = func(ip string) (string, bool, error) {
		switch ip {
		case "192.168.1.50", "192.168.1.52":
			return "http://192.168.1.10:8066/stream/party01.mp3?sig=1.a", true, nil
		case "192.168.1.51":
			return "http://192.168.1.10:8066/stream/house.mp3?sig=1.a", true, nil
		}
		return "", false, nil
	}
	var mu sync.Mutex
	stopped := map[string]bool{}
	srv.stopCast = func(ip string) error {
		mu.Lock()
		defer mu.Unlock()
		stopped[ip] = true
		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/streams/party01", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !stopped["192.168.1.50"] || !stopped["192.168.1.52"] {
		t.Errorf("speakers playing the deleted stream were not stopped: %v", stopped)
	}
	if stopped["192.168.1.51"] {
		t.Errorf("a speaker on a different stream must be left alone: %v", stopped)
	}
	if _, ok, _ := store.GetStream(db, "party01"); ok {
		t.Errorf("stream row should be gone after the delete")
	}
}

// An unreachable speaker must not block the delete.
func TestDeleteStreamSurvivesAnUnreachableSpeaker(t *testing.T) {
	srv, db := newTestServer(t)
	store.CreateSharedStream(db, "party01", "Party")
	srv.rememberDevices([]sonos.Device{{Name: "Kitchen", IP: "192.168.1.50"}})
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.deviceURI = func(string) (string, bool, error) {
		return "http://192.168.1.10:8066/stream/party01.mp3", true, nil
	}
	srv.stopCast = func(string) error { return http.ErrHandlerTimeout }

	req := httptest.NewRequest(http.MethodDelete, "/api/streams/party01", nil)
	req.AddCookie(adminSession(t, db))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: want 200 despite an unreachable speaker, got %d (%s)", rec.Code, rec.Body.String())
	}
	if _, ok, _ := store.GetStream(db, "party01"); ok {
		t.Errorf("stream row should be gone after the delete")
	}
}

// The device reads must actually overlap: each one is a round trip to a speaker
// that may be asleep, and four sequential 4-second timeouts would be a
// sixteen-second device list. The barrier makes serialized reads fail rather
// than merely be slow — no read can return until every read has started.
func TestDeviceListReadsSpeakersConcurrently(t *testing.T) {
	srv, _ := newTestServer(t)
	const n = 4
	devs := make([]sonos.Device, n)
	for i := range devs {
		devs[i] = sonos.Device{Name: fmt.Sprintf("Speaker %d", i), IP: fmt.Sprintf("192.168.1.5%d", i)}
	}
	srv.sonosDiscover = func() ([]sonos.Device, error) { return devs, nil }

	arrived := make(chan struct{}, n)
	release := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			<-arrived
		}
		close(release)
	}()
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.deviceURI = func(string) (string, bool, error) {
		arrived <- struct{}{}
		select {
		case <-release:
		case <-time.After(5 * time.Second):
			return "", false, fmt.Errorf("read did not overlap the others: the device reads are serialized")
		}
		return "http://192.168.1.10:8066/stream/house.mp3?sig=1.a", true, nil
	}

	for _, d := range castDevicesJSON(t, srv) {
		if d["stream"] != "house" {
			t.Fatalf("device %v stream = %v, want house", d["ip"], d["stream"])
		}
	}
}

// A speaker playing ANOTHER jukebox's stream is not playing ours. Every install
// serves /stream/house.mp3, so the path shape alone cannot tell two servers on
// one LAN apart — and for a stream id that collided, our delete would have
// stopped their speaker.
func TestDeviceListIgnoresAnotherServersStream(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.sonosDiscover = func() ([]sonos.Device, error) {
		return []sonos.Device{
			{Name: "Kitchen", IP: "192.168.1.50"},
			{Name: "Patio", IP: "192.168.1.51"},
		}, nil
	}
	srv.deviceURI = func(ip string) (string, bool, error) {
		if ip == "192.168.1.50" {
			return "http://192.168.1.99:8066/stream/house.mp3?sig=1.a", true, nil // the other jukebox
		}
		return "http://192.168.1.10:8066/stream/house.mp3?sig=1.a", true, nil // ours
	}
	got := map[string]any{}
	for _, d := range castDevicesJSON(t, srv) {
		got[d["ip"].(string)] = d["stream"]
	}
	if got["192.168.1.50"] != nil {
		t.Errorf("a speaker on another server's house stream = %v, want nil", got["192.168.1.50"])
	}
	if got["192.168.1.51"] != "house" {
		t.Errorf("a speaker on our own house stream = %v, want house", got["192.168.1.51"])
	}
}

// Deleting a stream must not reach across to a speaker playing the same path on
// a different server.
func TestDeleteStreamLeavesAnotherServersSpeakerAlone(t *testing.T) {
	srv, db := newTestServer(t)
	store.CreateSharedStream(db, "party01", "Party")
	srv.hostIPs = func() []string { return []string{"192.168.1.10"} }
	srv.rememberDevices([]sonos.Device{
		{Name: "Kitchen", IP: "192.168.1.50"},
		{Name: "Patio", IP: "192.168.1.51"},
	})
	srv.deviceURI = func(ip string) (string, bool, error) {
		if ip == "192.168.1.50" {
			return "http://192.168.1.99:8066/stream/party01.mp3", true, nil // not ours
		}
		return "http://192.168.1.10:8066/stream/party01.mp3", true, nil
	}
	var mu sync.Mutex
	stopped := map[string]bool{}
	srv.stopCast = func(ip string) error {
		mu.Lock()
		defer mu.Unlock()
		stopped[ip] = true
		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/streams/party01", nil)
	req.AddCookie(adminSession(t, db))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if stopped["192.168.1.50"] {
		t.Errorf("stopped a speaker playing another server's stream: %v", stopped)
	}
	if !stopped["192.168.1.51"] {
		t.Errorf("our own speaker was not stopped: %v", stopped)
	}
}

// Interfaces that cannot be read must not silently switch the mapping off: the
// check fails open, back to the path-shape answer it replaced.
func TestDeviceStreamFailsOpenWhenHostAddressesAreUnknown(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.hostIPs = func() []string { return nil }
	srv.deviceURI = func(string) (string, bool, error) {
		return "http://192.168.1.99:8066/stream/house.mp3", true, nil
	}
	if got := srv.deviceStream("192.168.1.50"); got != "house" {
		t.Fatalf("deviceStream = %q, want house when the host's own addresses are unknown", got)
	}
}
