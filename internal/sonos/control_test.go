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

func TestRenderingControlURL(t *testing.T) {
	if got := RenderingControlURL("10.0.0.5"); got != "http://10.0.0.5:1400/MediaRenderer/RenderingControl/Control" {
		t.Fatalf("RenderingControlURL = %q", got)
	}
}

func TestSetVolumeSendsEnvelope(t *testing.T) {
	var action, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action = r.Header.Get("SOAPACTION")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := setVolumeURL(srv.URL, 42); err != nil {
		t.Fatalf("setVolumeURL: %v", err)
	}
	if !strings.Contains(action, renderingControl+"#SetVolume") {
		t.Fatalf("SOAPACTION = %q, want RenderingControl#SetVolume", action)
	}
	if !strings.Contains(body, "<Channel>Master</Channel>") {
		t.Fatalf("body missing Master channel: %q", body)
	}
	if !strings.Contains(body, "<DesiredVolume>42</DesiredVolume>") {
		t.Fatalf("body missing DesiredVolume: %q", body)
	}
}

func TestGetVolumeParsesResponse(t *testing.T) {
	var action string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action = r.Header.Get("SOAPACTION")
		io.WriteString(w, `<?xml version="1.0"?><s:Envelope><s:Body>`+
			`<u:GetVolumeResponse><CurrentVolume>37</CurrentVolume></u:GetVolumeResponse>`+
			`</s:Body></s:Envelope>`)
	}))
	defer srv.Close()
	v, err := getVolumeURL(srv.URL)
	if err != nil {
		t.Fatalf("getVolumeURL: %v", err)
	}
	if v != 37 {
		t.Fatalf("getVolumeURL = %d, want 37", v)
	}
	if !strings.Contains(action, renderingControl+"#GetVolume") {
		t.Fatalf("SOAPACTION = %q, want RenderingControl#GetVolume", action)
	}
}
