package fed

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/andybarilla/exit66jukebox/internal/model"
	"github.com/andybarilla/exit66jukebox/internal/store"
)

func TestEndToEndCatalogSync(t *testing.T) {
	homeDB, _ := store.Open(":memory:")
	defer homeDB.Close()
	vpsDB, _ := store.Open(":memory:")
	defer vpsDB.Close()
	hubDB, _ := store.Open(":memory:")
	defer hubDB.Close()
	if _, err := store.UpsertTrack(homeDB, model.Track{Path: "/m/x.mp3", Title: "Hit", TrackNo: 1}, "Band", "Band", "Rec"); err != nil {
		t.Fatal(err)
	}

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	reg := NewRegistry()
	relay := NewRelay(reg, hubDB)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				p, err := acceptAndRegister(conn, "tok", reg)
				if err != nil {
					return
				}
				http.Serve(p.Session, relay.Routes())
			}()
		}
	}()

	home := &Manager{Role: "member", Token: "tok", PeerID: "home", HubAddr: ln.Addr().String(), Registry: NewRegistry()}
	home.Start()
	for i := 0; i < 300 && home.HubClient() == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if err := PushCatalog(homeDB, home.HubClient(), "home"); err != nil {
		t.Fatal(err)
	}

	vps := &Manager{Role: "member", Token: "tok", PeerID: "vps", HubAddr: ln.Addr().String(), Registry: NewRegistry()}
	vps.Start()
	for i := 0; i < 300 && vps.HubClient() == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	online, err := PullAndApply(vpsDB, vps.HubClient(), "vps")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := store.ListTracks(vpsDB, "", 0, 0)
	if len(got) != 1 || got[0].Title != "Hit" {
		t.Fatalf("expected home's track synced to vps db, got %+v", got)
	}
	foundHome := false
	for _, p := range online {
		if p == "home" {
			foundHome = true
		}
	}
	if !foundHome {
		t.Fatalf("expected 'home' in online peers, got %v", online)
	}
}
