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
