package sonos

import (
	"net"
	"strconv"
	"sync"
	"time"
)

// scanConcurrency caps in-flight unicast probes during a /24 sweep.
const scanConcurrency = 32

// ScanUnicast sweeps the /24 of hostIP for Sonos players answering on :1400,
// confirming each via its device descriptor. It is the unicast fallback for
// networks where SSDP multicast discovery returns nothing. Returns nil if hostIP
// has no usable /24. Results are ordered by ascending host octet.
func ScanUnicast(hostIP string, dialTimeout time.Duration) []Device {
	prefix, ok := subnet24(hostIP)
	if !ok {
		return nil
	}
	return scanSubnet(prefix, scanConcurrency, func(ip string) (Device, bool) {
		return probeSonos(ip, dialTimeout)
	})
}

// subnet24 returns the dotted /24 prefix (e.g. "192.168.1.") of an IPv4 address.
// ok is false for empty, malformed, or non-IPv4 input.
func subnet24(ip string) (string, bool) {
	v4 := net.ParseIP(ip).To4()
	if v4 == nil {
		return "", false
	}
	return strconv.Itoa(int(v4[0])) + "." + strconv.Itoa(int(v4[1])) + "." + strconv.Itoa(int(v4[2])) + ".", true
}

// scanSubnet probes hosts .1–.254 of prefix concurrently (at most cap in flight)
// using probe, returning the verified devices ordered by ascending host octet.
func scanSubnet(prefix string, cap int, probe func(ip string) (Device, bool)) []Device {
	results := make([]Device, 255) // index by host octet; .0 unused
	found := make([]bool, 255)
	sem := make(chan struct{}, cap)
	var wg sync.WaitGroup
	for host := 1; host <= 254; host++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(host int) {
			defer wg.Done()
			defer func() { <-sem }()
			if d, ok := probe(prefix + strconv.Itoa(host)); ok {
				results[host] = d
				found[host] = true
			}
		}(host)
	}
	wg.Wait()

	devices := make([]Device, 0)
	for host := 1; host <= 254; host++ {
		if found[host] {
			devices = append(devices, results[host])
		}
	}
	return devices
}

// probeSonos dials ip:1400 with a short timeout; only on a successful connect
// does it fetch the device descriptor (the slower step). Returns the Sonos device
// if the host serves a valid descriptor.
func probeSonos(ip string, dialTimeout time.Duration) (Device, bool) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "1400"), dialTimeout)
	if err != nil {
		return Device{}, false
	}
	conn.Close()
	name, ok := Verify(DescriptorURL(ip))
	if !ok {
		return Device{}, false
	}
	return Device{Name: name, IP: ip}, true
}
