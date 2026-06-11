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
