package email

import (
	"net/smtp"
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

func TestSendPasswordResetMessage(t *testing.T) {
	s := New(Config{Host: "smtp.example.com", Port: "587", From: "jukebox@host"})
	var sent string
	s.send = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		sent = string(msg)
		return nil
	}
	if err := s.SendPasswordReset("to@host", "https://hub/reset-password/abc"); err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}
	if !strings.Contains(sent, "Subject: Reset your Exit 66 Jukebox password") {
		t.Fatalf("reset subject missing: %s", sent)
	}
	if !strings.Contains(sent, "https://hub/reset-password/abc") {
		t.Fatalf("reset link missing: %s", sent)
	}
}

func TestSendInviteRejectsHeaderInjection(t *testing.T) {
	s := New(Config{Host: "smtp.example.com", Port: "587", From: "jukebox@host"})
	called := false
	s.send = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		called = true
		return nil
	}
	if err := s.SendInvite("victim@host\r\nBcc: evil@host", "https://hub/invite/abc"); err == nil {
		t.Fatal("CRLF in recipient accepted")
	}
	if called {
		t.Fatal("send invoked despite invalid recipient")
	}
}
