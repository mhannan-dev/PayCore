package mailer

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a minimal SMTP server on 127.0.0.1 for exercising the client.
type fakeSMTP struct {
	extensions []string // advertised after EHLO, e.g. "AUTH LOGIN"
	mu         sync.Mutex
	user, pass string
	data       string
	commands   []string
}

func (f *fakeSMTP) start(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return ln.Addr().(*net.TCPAddr).AddrPort().String()
}

func (f *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	reply := func(s string) { conn.Write([]byte(s + "\r\n")) }
	readLine := func() (string, bool) {
		line, err := r.ReadString('\n')
		return strings.TrimRight(line, "\r\n"), err == nil
	}
	reply("220 fake ESMTP")
	for {
		line, ok := readLine()
		if !ok {
			return
		}
		f.mu.Lock()
		f.commands = append(f.commands, strings.Fields(line + " ")[0])
		f.mu.Unlock()
		switch cmd := strings.ToUpper(strings.Fields(line + " ")[0]); cmd {
		case "EHLO":
			lines := append([]string{"fake"}, f.extensions...)
			for i, l := range lines {
				sep := "-"
				if i == len(lines)-1 {
					sep = " "
				}
				reply("250" + sep + l)
			}
		case "AUTH":
			reply("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
			u, _ := readLine()
			reply("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
			p, _ := readLine()
			du, _ := base64.StdEncoding.DecodeString(u)
			dp, _ := base64.StdEncoding.DecodeString(p)
			f.mu.Lock()
			f.user, f.pass = string(du), string(dp)
			f.mu.Unlock()
			reply("235 ok")
		case "MAIL", "RCPT", "RSET", "NOOP":
			reply("250 ok")
		case "DATA":
			reply("354 go ahead")
			var b strings.Builder
			for {
				l, ok := readLine()
				if !ok || l == "." {
					break
				}
				b.WriteString(l + "\n")
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			reply("250 queued")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 unknown")
		}
	}
}

func split(addr string) (string, string) {
	host, port, _ := net.SplitHostPort(addr)
	return host, port
}

func TestSMTPAuthLoginAndQuotedPrintable(t *testing.T) {
	f := &fakeSMTP{extensions: []string{"AUTH LOGIN XOAUTH2"}}
	host, port := split(f.start(t))
	s := SMTP{Host: host, Port: port, Username: "mailbox@example.test", Password: "s3cret", From: "PayCore <mailbox@example.test>", Encryption: EncryptionNone}
	long := strings.Repeat("x", 1500) + " é"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Send(ctx, Message{To: "user@example.test", Subject: "Hi", Text: long, HTML: "<p>" + long + "</p>"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.user != "mailbox@example.test" || f.pass != "s3cret" {
		t.Fatalf("AUTH LOGIN credentials: %q %q", f.user, f.pass)
	}
	for _, line := range strings.Split(f.data, "\n") {
		if len(line) > 998 {
			t.Fatalf("line of %d bytes exceeds the SMTP limit", len(line))
		}
	}
	for _, want := range []string{"Message-ID: <", "@example.test>", "Content-Transfer-Encoding: quoted-printable", "=C3=A9"} {
		if !strings.Contains(f.data, want) {
			t.Errorf("message lacks %q", want)
		}
	}
}

func TestSMTPRequiresSTARTTLS(t *testing.T) {
	f := &fakeSMTP{extensions: []string{"AUTH LOGIN"}}
	host, port := split(f.start(t))
	s := SMTP{Host: host, Port: port, Username: "u@example.test", Password: "p", From: "u@example.test", Encryption: EncryptionSTARTTLS}
	err := s.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected a STARTTLS refusal, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.user != "" || f.pass != "" {
		t.Fatal("credentials must not be sent without TLS")
	}
}

func TestSMTPValidate(t *testing.T) {
	cases := map[string]SMTP{
		"no host":          {Port: "587", From: "a@b.test", Encryption: EncryptionSTARTTLS},
		"bad encryption":   {Host: "smtp.example.test", Port: "587", From: "a@b.test", Encryption: "maybe"},
		"remote plaintext": {Host: "smtp.example.test", Port: "25", From: "a@b.test", Encryption: EncryptionNone},
		"bad from":         {Host: "smtp.example.test", Port: "587", From: "not an address", Encryption: EncryptionSTARTTLS},
	}
	for name, s := range cases {
		if s.Validate() == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if err := (SMTP{Host: "127.0.0.1", Port: "1025", From: "a@b.test", Encryption: EncryptionNone}).Validate(); err != nil {
		t.Errorf("local Mailpit config: %v", err)
	}
}

func TestSMTPContextTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // accepts but never greets
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer c.Close()
			time.Sleep(3 * time.Second)
		}
	}()
	host, port := split(ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = SMTP{Host: host, Port: port, From: "a@b.test", Encryption: EncryptionNone}.Check(ctx)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("expected a prompt timeout, got %v after %s", err, time.Since(start))
	}
}
