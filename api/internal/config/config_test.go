package config

import "testing"

func TestMFAEnabledDefaultsAndExplicitPause(t *testing.T) {
	for _, tc := range []struct {
		value   string
		enabled bool
		invalid bool
	}{{"", true, false}, {"true", true, false}, {"false", false, false}, {" false ", false, false}, {"typo", false, true}} {
		got, err := parseMFAEnabled(tc.value)
		if (err != nil) != tc.invalid || (!tc.invalid && got != tc.enabled) {
			t.Fatalf("%q got %v %v", tc.value, got, err)
		}
	}
}

func clearMail(t *testing.T) {
	for _, k := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_ENCRYPTION",
		"MAIL_HOST", "MAIL_PORT", "MAIL_USERNAME", "MAIL_PASSWORD", "MAIL_ENCRYPTION", "MAIL_FROM_ADDRESS", "MAIL_FROM_NAME"} {
		t.Setenv(k, "")
	}
}

func TestLoadSMTPFromLaravelVariables(t *testing.T) {
	clearMail(t)
	t.Setenv("MAIL_HOST", "smtp.office365.com")
	t.Setenv("MAIL_PORT", "587")
	t.Setenv("MAIL_USERNAME", "box@corp.test")
	t.Setenv("MAIL_PASSWORD", "pw")
	t.Setenv("MAIL_ENCRYPTION", "starttls")
	t.Setenv("MAIL_FROM_ADDRESS", "box@corp.test")
	t.Setenv("MAIL_FROM_NAME", "${APP_NAME}") // unexpanded/empty in .env → default name
	var c Config
	loadSMTP(&c)
	if c.SMTPSource != "MAIL_*" || c.SMTPHost != "smtp.office365.com" || c.SMTPPort != "587" || c.SMTPEncryption != "starttls" || c.SMTPUsername != "box@corp.test" || c.SMTPPassword != "pw" {
		t.Fatalf("%+v", c)
	}
	if c.SMTPFrom != `"PayCore" <box@corp.test>` {
		t.Fatalf("from: %s", c.SMTPFrom)
	}
}

func TestSMTPVariablesWinAndAreNotMixed(t *testing.T) {
	clearMail(t)
	t.Setenv("SMTP_HOST", "127.0.0.1")
	t.Setenv("SMTP_PORT", "1025")
	t.Setenv("MAIL_HOST", "smtp.office365.com")
	t.Setenv("MAIL_USERNAME", "box@corp.test")
	t.Setenv("MAIL_PASSWORD", "pw")
	var c Config
	loadSMTP(&c)
	if c.SMTPHost != "127.0.0.1" || c.SMTPUsername != "" || c.SMTPPassword != "" || c.SMTPEncryption != "none" {
		t.Fatalf("MAIL_* credentials leaked into the SMTP_* server: %+v", c)
	}
	if c.SMTPFrom != `"PayCore" <no-reply@paycore.local>` {
		t.Fatalf("from: %s", c.SMTPFrom)
	}
}

func TestEncryptionDefaults(t *testing.T) {
	cases := []struct{ value, host, port, want string }{
		{"", "smtp.example.test", "587", "starttls"},
		{"", "smtp.example.test", "465", "tls"},
		{"tls", "smtp.example.test", "587", "starttls"}, // Laravel's "tls" on 587 means STARTTLS
		{"ssl", "smtp.example.test", "465", "tls"},
		{"", "localhost", "1025", "none"},
		{"null", "127.0.0.1", "1025", "none"},
	}
	for _, c := range cases {
		if got := normalizeEncryption(c.value, c.host, c.port); got != c.want {
			t.Errorf("%q %s:%s → %s, want %s", c.value, c.host, c.port, got, c.want)
		}
	}
}

func TestNoMailServer(t *testing.T) {
	clearMail(t)
	var c Config
	loadSMTP(&c)
	if c.SMTPHost != "" {
		t.Fatalf("%+v", c)
	}
}
