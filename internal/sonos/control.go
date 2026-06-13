package sonos

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	avTransport      = "urn:schemas-upnp-org:service:AVTransport:1"
	renderingControl = "urn:schemas-upnp-org:service:RenderingControl:1"
)

// soapClient is reused across SOAP calls so SetAVTransportURI + Play to the same
// player can share a connection.
var soapClient = &http.Client{Timeout: 4 * time.Second}

// ControlURL is the fixed AVTransport control endpoint for a Sonos player.
func ControlURL(ip string) string {
	return fmt.Sprintf("http://%s:1400/MediaRenderer/AVTransport/Control", ip)
}

// RenderingControlURL is the fixed RenderingControl endpoint for a Sonos player,
// where volume lives (a different service + path than AVTransport).
func RenderingControlURL(ip string) string {
	return fmt.Sprintf("http://%s:1400/MediaRenderer/RenderingControl/Control", ip)
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
	if err := soap(controlURL, avTransport, "SetAVTransportURI", set); err != nil {
		return err
	}
	play := fmt.Sprintf(`<u:Play xmlns:u="%s"><InstanceID>0</InstanceID><Speed>1</Speed></u:Play>`, avTransport)
	return soap(controlURL, avTransport, "Play", play)
}

func stopURL(controlURL string) error {
	body := fmt.Sprintf(`<u:Stop xmlns:u="%s"><InstanceID>0</InstanceID></u:Stop>`, avTransport)
	return soap(controlURL, avTransport, "Stop", body)
}

// SetVolume sets the master playback volume (0–100) on the Sonos at ip.
func SetVolume(ip string, vol int) error { return setVolumeURL(RenderingControlURL(ip), vol) }

// GetVolume reads the master playback volume (0–100) from the Sonos at ip.
func GetVolume(ip string) (int, error) { return getVolumeURL(RenderingControlURL(ip)) }

func setVolumeURL(controlURL string, vol int) error {
	body := fmt.Sprintf(
		`<u:SetVolume xmlns:u="%s"><InstanceID>0</InstanceID>`+
			`<Channel>Master</Channel><DesiredVolume>%d</DesiredVolume></u:SetVolume>`,
		renderingControl, vol)
	return soap(controlURL, renderingControl, "SetVolume", body)
}

var currentVolumeRe = regexp.MustCompile(`<CurrentVolume>(\d+)</CurrentVolume>`)

func getVolumeURL(controlURL string) (int, error) {
	body := fmt.Sprintf(
		`<u:GetVolume xmlns:u="%s"><InstanceID>0</InstanceID><Channel>Master</Channel></u:GetVolume>`,
		renderingControl)
	resp, err := soapCall(controlURL, renderingControl, "GetVolume", body)
	if err != nil {
		return 0, err
	}
	m := currentVolumeRe.FindSubmatch(resp)
	if m == nil {
		return 0, fmt.Errorf("sonos GetVolume: no CurrentVolume in response")
	}
	return strconv.Atoi(string(m[1]))
}

// soap sends a SOAP action and discards the response body.
func soap(controlURL, service, action, inner string) error {
	_, err := soapCall(controlURL, service, action, inner)
	return err
}

// soapCall sends a SOAP action and returns the response body. service is the
// UPnP service URN the action belongs to (AVTransport, RenderingControl, …),
// which sets both the SOAPACTION header and must match the control path.
func soapCall(controlURL, service, action, inner string) ([]byte, error) {
	env := `<?xml version="1.0"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" ` +
		`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		inner + `</s:Body></s:Envelope>`
	req, err := http.NewRequest(http.MethodPost, controlURL, strings.NewReader(env))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"`+service+`#`+action+`"`)
	resp, err := soapClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sonos %s: status %d", action, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64*1024))
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
