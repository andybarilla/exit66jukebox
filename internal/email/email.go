// Package email sends invite emails over SMTP. It is entirely optional: with no
// SMTP host configured the Sender is disabled and the app surfaces invite links
// for manual sharing instead.
package email

import (
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"
)

// Config holds SMTP settings, read from the environment by main.
type Config struct {
	Host string // e.g. "smtp.example.com"
	Port string // e.g. "587"
	User string
	Pass string
	From string // envelope + header From
}

// Sender sends invite emails. The zero/unconfigured value is disabled.
type Sender struct {
	cfg  Config
	send func(addr string, a smtp.Auth, from string, to []string, msg []byte) error // injectable for tests
}

// New builds a Sender from cfg. When Host is empty the Sender is disabled.
func New(cfg Config) *Sender {
	return &Sender{cfg: cfg, send: smtp.SendMail}
}

// Enabled reports whether SMTP is configured.
func (s *Sender) Enabled() bool { return s.cfg.Host != "" }

// SendInvite emails the invite link to addr. No-op (nil) when disabled.
func (s *Sender) SendInvite(to, link string) error {
	return s.sendLink(to, link, inviteMessage)
}

func (s *Sender) SendPasswordReset(to, link string) error {
	return s.sendLink(to, link, passwordResetMessage)
}

func (s *Sender) SendVerification(to, link string) error {
	return s.sendLink(to, link, verificationMessage)
}

func (s *Sender) sendLink(to, link string, buildMessage func(from, to, link string) []byte) error {
	if !s.Enabled() {
		return nil
	}
	// Guard against header injection: a recipient, link, or From containing CR/LF
	// could inject extra SMTP headers (e.g. a hidden Bcc). Validate the address
	// and reject control characters before building the message.
	if _, err := mail.ParseAddress(to); err != nil {
		return fmt.Errorf("invalid recipient address: %w", err)
	}
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(link, "\r\n") || strings.ContainsAny(s.cfg.From, "\r\n") {
		return fmt.Errorf("invalid header value")
	}
	var auth smtp.Auth
	if s.cfg.User != "" {
		auth = smtp.PlainAuth("", s.cfg.User, s.cfg.Pass, s.cfg.Host)
	}
	addr := s.cfg.Host + ":" + s.cfg.Port
	return s.send(addr, auth, s.cfg.From, []string{to}, buildMessage(s.cfg.From, to, link))
}

// inviteMessage builds an RFC 5322 message.
func inviteMessage(from, to, link string) []byte {
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: You're invited to Exit 66 Jukebox\r\n\r\n"+
			"You've been invited. Open this link to set up your account:\r\n\r\n%s\r\n",
		from, to, link))
}

func passwordResetMessage(from, to, link string) []byte {
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Reset your Exit 66 Jukebox password\r\n\r\n"+
			"Open this link within 1 hour to set a new password:\r\n\r\n%s\r\n",
		from, to, link))
}

func verificationMessage(from, to, link string) []byte {
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: Verify your Exit 66 Jukebox email\r\n\r\n"+
			"Open this link to verify your email address:\r\n\r\n%s\r\n",
		from, to, link))
}
