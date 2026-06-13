package sonos

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubnet24(t *testing.T) {
	cases := []struct {
		ip     string
		prefix string
		ok     bool
	}{
		{"192.168.1.50", "192.168.1.", true},
		{"10.0.0.7", "10.0.0.", true},
		{"172.16.5.200", "172.16.5.", true},
		{"", "", false},
		{"not-an-ip", "", false},
		{"::1", "", false},
		{"2001:db8::1", "", false},
	}
	for _, c := range cases {
		prefix, ok := subnet24(c.ip)
		if prefix != c.prefix || ok != c.ok {
			t.Errorf("subnet24(%q) = (%q, %v), want (%q, %v)", c.ip, prefix, ok, c.prefix, c.ok)
		}
	}
}

func TestScanSubnetReturnsVerifiedDevices(t *testing.T) {
	// A real Sonos descriptor server and a non-Sonos server. The probe seam wires
	// scanSubnet's per-host check to actual Verify calls against these.
	sonosSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, sampleDesc)
	}))
	defer sonosSrv.Close()
	nonSonosSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<html>not a sonos</html>")
	}))
	defer nonSonosSrv.Close()

	// Map a couple of host octets onto the mock servers; everything else is dead.
	probe := func(ip string) (Device, bool) {
		switch ip {
		case "192.168.1.10":
			if name, ok := Verify(sonosSrv.URL); ok {
				return Device{Name: name, IP: ip}, true
			}
		case "192.168.1.20":
			if name, ok := Verify(nonSonosSrv.URL); ok {
				return Device{Name: name, IP: ip}, true
			}
		}
		return Device{}, false
	}

	got := scanSubnet("192.168.1.", 32, probe)
	if len(got) != 1 {
		t.Fatalf("scanSubnet returned %d devices, want 1: %+v", len(got), got)
	}
	if got[0].IP != "192.168.1.10" || got[0].Name != "Living Room" {
		t.Fatalf("scanSubnet = %+v, want {Living Room 192.168.1.10}", got[0])
	}
}

func TestScanSubnetScansFullHostRange(t *testing.T) {
	var mu sync.Mutex
	var seen []int
	probe := func(ip string) (Device, bool) {
		mu.Lock()
		seen = append(seen, 1)
		mu.Unlock()
		return Device{}, false
	}
	scanSubnet("10.0.0.", 32, probe)
	if len(seen) != 254 {
		t.Fatalf("probe called %d times, want 254 (.1-.254)", len(seen))
	}
}

func TestScanSubnetRespectsConcurrencyCap(t *testing.T) {
	const cap = 8
	var inFlight, maxSeen int32
	probe := func(ip string) (Device, bool) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			m := atomic.LoadInt32(&maxSeen)
			if n <= m || atomic.CompareAndSwapInt32(&maxSeen, m, n) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return Device{}, false
	}
	scanSubnet("10.0.0.", cap, probe)
	if maxSeen > cap {
		t.Fatalf("max concurrency %d exceeded cap %d", maxSeen, cap)
	}
	if maxSeen == 0 {
		t.Fatalf("probe never ran")
	}
}

func TestScanSubnetOrderedByHostOctet(t *testing.T) {
	probe := func(ip string) (Device, bool) {
		switch ip {
		case "10.0.0.5", "10.0.0.100", "10.0.0.30":
			return Device{Name: ip, IP: ip}, true
		}
		return Device{}, false
	}
	got := scanSubnet("10.0.0.", 32, probe)
	ips := make([]string, len(got))
	for i, d := range got {
		ips[i] = d.IP
	}
	want := []string{"10.0.0.5", "10.0.0.30", "10.0.0.100"}
	if len(got) != len(want) {
		t.Fatalf("got %d devices, want %d: %v", len(got), len(want), ips)
	}
	for i := range want {
		if got[i].IP != want[i] {
			t.Fatalf("scanSubnet order = %v, want %v", ips, want)
		}
	}
}
