package sonos

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestDescriptorURL(t *testing.T) {
	if got := DescriptorURL("10.0.0.7"); got != "http://10.0.0.7:1400/xml/device_description.xml" {
		t.Fatalf("DescriptorURL = %q", got)
	}
}

func TestVerifyReturnsRoomName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, sampleDesc)
	}))
	defer srv.Close()
	name, ok := Verify(srv.URL)
	if !ok || name != "Living Room" {
		t.Fatalf("Verify = (%q, %v), want (Living Room, true)", name, ok)
	}
}

func TestVerifyRejectsNonSonos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<html>not a sonos</html>")
	}))
	defer srv.Close()
	if name, ok := Verify(srv.URL); ok {
		t.Fatalf("Verify of non-Sonos = (%q, true), want ok=false", name)
	}
}
