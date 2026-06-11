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
