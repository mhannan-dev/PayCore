// Package mailer sends transactional email (plain text plus HTML).
package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"slices"
	"strings"
	"time"
)

// Message is one email. To must be a bare address; Subject must be a single line.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers a Message.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Encryption modes for SMTP.
const (
	EncryptionSTARTTLS = "starttls" // plain connect, then upgrade; required (port 587)
	EncryptionTLS      = "tls"      // TLS from the first byte (port 465)
	EncryptionNone     = "none"     // only for local test servers such as Mailpit
)

// DefaultTimeout bounds one delivery when the caller's context has no deadline.
const DefaultTimeout = 30 * time.Second

// SMTP delivers through an SMTP server. Encryption is enforced, not opportunistic:
// with "starttls" a server that does not offer STARTTLS is an error. Credentials are
// only sent over TLS (or to a loopback server), using AUTH PLAIN or AUTH LOGIN
// (Microsoft 365 offers LOGIN but not PLAIN).
type SMTP struct {
	Host, Port, Username, Password, From string
	Encryption                           string
	// HelloName is sent in EHLO; defaults to "localhost".
	HelloName string
}

// mode is the effective encryption: an empty Encryption means TLS on port 465,
// none for a loopback server, otherwise STARTTLS.
func (s SMTP) mode() string {
	switch {
	case s.Encryption != "":
		return s.Encryption
	case s.Port == "465":
		return EncryptionTLS
	case isLoopback(s.Host):
		return EncryptionNone
	default:
		return EncryptionSTARTTLS
	}
}

// Validate reports configuration problems without connecting.
func (s SMTP) Validate() error {
	s.Encryption = s.mode()
	switch {
	case s.Host == "" || s.Port == "":
		return errors.New("SMTP host and port are required")
	case s.Encryption != EncryptionSTARTTLS && s.Encryption != EncryptionTLS && s.Encryption != EncryptionNone:
		return fmt.Errorf("SMTP encryption %q must be starttls, tls or none", s.Encryption)
	case s.Encryption == EncryptionNone && !isLoopback(s.Host):
		return errors.New("SMTP encryption \"none\" is only allowed for a local server (localhost / 127.0.0.1)")
	}
	if _, err := mail.ParseAddress(s.From); err != nil {
		return fmt.Errorf("invalid sender address %q: %w", s.From, err)
	}
	return nil
}

// Send delivers m. It returns once the server has accepted the message for delivery.
func (s SMTP) Send(ctx context.Context, m Message) error {
	if err := s.Validate(); err != nil {
		return err
	}
	from, _ := mail.ParseAddress(s.From)
	if strings.ContainsAny(m.To+m.Subject, "\r\n") {
		return errors.New("header values must be single-line")
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	body, err := build(from, to.Address, m)
	if err != nil {
		return err
	}
	return s.session(ctx, func(c *smtp.Client) error {
		if err := c.Mail(from.Address); err != nil {
			return fmt.Errorf("smtp MAIL FROM: %w", err)
		}
		if err := c.Rcpt(to.Address); err != nil {
			return fmt.Errorf("smtp RCPT TO: %w", err)
		}
		w, err := c.Data()
		if err != nil {
			return fmt.Errorf("smtp DATA: %w", err)
		}
		if _, err := w.Write(body); err != nil {
			return fmt.Errorf("smtp DATA: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("smtp DATA: %w", err) // the server's final verdict on the message
		}
		return nil
	})
}

// Check connects, negotiates encryption and authenticates, then quits without sending.
func (s SMTP) Check(ctx context.Context) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return s.session(ctx, func(*smtp.Client) error { return nil })
}

// session dials, upgrades to TLS, authenticates, runs fn and quits. The context
// bounds the whole conversation; cancelling it closes the connection.
func (s SMTP) session(ctx context.Context, fn func(*smtp.Client) error) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	s.Encryption = s.mode()
	addr := net.JoinHostPort(s.Host, s.Port)
	tlsConfig := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	dialer := &net.Dialer{}
	var conn net.Conn
	var err error
	if s.Encryption == EncryptionTLS {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp connect %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp greeting: %w", err)
	}
	defer c.Close()
	hello := s.HelloName
	if hello == "" {
		hello = "localhost"
	}
	if err := c.Hello(hello); err != nil {
		return fmt.Errorf("smtp EHLO: %w", err)
	}
	if s.Encryption == EncryptionSTARTTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("smtp: server does not offer STARTTLS; refusing to continue unencrypted")
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("smtp STARTTLS: %w", err)
		}
	}
	if s.Username != "" {
		ok, mechanisms := c.Extension("AUTH")
		if !ok {
			return errors.New("smtp: server does not offer AUTH but a username is configured")
		}
		if err := c.Auth(s.auth(strings.Fields(strings.ToUpper(mechanisms)))); err != nil {
			return fmt.Errorf("smtp AUTH (user %s): %w", s.Username, err)
		}
	}
	if err := fn(c); err != nil {
		return err
	}
	if err := c.Quit(); err != nil && ctx.Err() == nil {
		// The message (if any) was already accepted at DATA; a failed QUIT is harmless.
		log.Printf("[mailer] smtp QUIT: %v", err)
	}
	return ctx.Err()
}

func (s SMTP) auth(mechanisms []string) smtp.Auth {
	if slices.Contains(mechanisms, "PLAIN") || !slices.Contains(mechanisms, "LOGIN") {
		return smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	return &loginAuth{username: s.Username, password: s.Password, host: s.Host}
}

// loginAuth implements AUTH LOGIN, refusing to send credentials without TLS
// (except to loopback), like net/smtp's PlainAuth.
type loginAuth struct{ username, password, host string }

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !isLoopback(server.Name) {
		return "", nil, errors.New("unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, errors.New("wrong host name")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch prompt := strings.ToLower(strings.TrimSpace(string(fromServer))); {
	case strings.HasPrefix(prompt, "user"):
		return []byte(a.username), nil
	case strings.HasPrefix(prompt, "pass"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected AUTH LOGIN prompt %q", fromServer)
	}
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// build renders a multipart/alternative message. Bodies are quoted-printable, so
// long lines and non-ASCII text stay within SMTP's 998-byte line limit.
func build(from *mail.Address, to string, m Message) ([]byte, error) {
	boundary, err := randomHex(12)
	if err != nil {
		return nil, err
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	domain := "paycore.local"
	if at := strings.LastIndex(from.Address, "@"); at >= 0 {
		domain = from.Address[at+1:]
	}
	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", from.String())
	header("To", to)
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", time.Now().Format(time.RFC1123Z))
	header("Message-ID", "<"+id+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", `multipart/alternative; boundary="paycore-`+boundary+`"`)
	b.WriteString("\r\n")
	for _, part := range []struct{ kind, content string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		if part.content == "" {
			continue
		}
		fmt.Fprintf(&b, "--paycore-%s\r\nContent-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", boundary, part.kind)
		qp := quotedprintable.NewWriter(&b)
		if _, err := qp.Write([]byte(part.content)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--paycore-%s--\r\n", boundary)
	return b.Bytes(), nil
}

// Log writes emails to the server log instead of sending them. Development only:
// the log then contains working one-time links.
type Log struct{}

// ErrNotDelivered distinguishes a development preview from SMTP acceptance.
var ErrNotDelivered = errors.New("email was logged for development but not delivered")

func (Log) Send(_ context.Context, m Message) error {
	log.Printf("[mailer] SMTP is not configured; email not sent. To: %s | Subject: %s\n%s", m.To, m.Subject, m.Text)
	return ErrNotDelivered
}
