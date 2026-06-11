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
