package email

import (
	"strings"
	"testing"
)

func TestInviteMessage(t *testing.T) {
	msg := inviteMessage("from@host", "to@host", "https://hub/invite/abc")
	s := string(msg)
	if !strings.Contains(s, "To: to@host") || !strings.Contains(s, "From: from@host") {
		t.Fatalf("headers missing: %s", s)
	}
	if !strings.Contains(s, "https://hub/invite/abc") {
		t.Fatal("link missing from body")
	}
	if !strings.Contains(s, "Subject:") {
		t.Fatal("subject missing")
	}
}

func TestSenderDisabledWhenUnconfigured(t *testing.T) {
	s := New(Config{}) // no host
	if s.Enabled() {
		t.Fatal("sender should be disabled with no host")
	}
}

func TestSendInviteDisabledIsNoop(t *testing.T) {
	s := New(Config{}) // disabled
	if err := s.SendInvite("x@y.com", "link"); err != nil {
		t.Fatalf("disabled SendInvite should be a no-op nil, got %v", err)
	}
}
