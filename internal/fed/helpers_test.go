package fed

import (
	"io"
	"log"
	"os"
	"testing"
)

// testLogger returns a logger prefixed for the running test. Set
// EXIT66_FED_DEBUG=1 to surface WebRTC transport logs while debugging.
func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	if os.Getenv("EXIT66_FED_DEBUG") != "" {
		return log.New(os.Stderr, "["+t.Name()+"] ", log.LstdFlags|log.Lmsgprefix)
	}
	return log.New(io.Discard, "", 0)
}
