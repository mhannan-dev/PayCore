package config

import (
	"log"
	"net"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	MFAEnabled       bool
	MFAEncryptionKey string `json:"-"`
	Port             string
	Environment      string
	DatabaseURL      string
	RedisURL         string
	UploadDir        string
	// AppURL is the web app's origin, used in emailed links (e.g. http://localhost:3000).
	AppURL string
	// SMTP settings for outgoing email. Empty SMTPHost: development logs emails instead.
	SMTPHost       string
	SMTPPort       string
	SMTPUsername   string
	SMTPPassword   string
	SMTPFrom       string
	SMTPEncryption string // starttls, tls or none
	// SMTPSource says which variable set was used ("SMTP_*" or "MAIL_*"), for the startup log.
	SMTPSource string
	// Outbox configures the background delivery of committed ledger events.
	Outbox OutboxConfig
}

// OutboxConfig selects the event publisher and its retry behaviour. The webhook
// sink can be a broker bridge or a consumer inbox; "log" needs no infrastructure.
type OutboxConfig struct {
	Publisher     string
	WebhookURL    string
	WebhookSecret string `json:"-"`
	BatchSize     int
	MaxRetries    int
	Interval      time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	mfaEnabled, err := parseMFAEnabled(os.Getenv("MFA_ENABLED"))
	if err != nil {
		log.Fatal("MFA_ENABLED must be true or false.")
	}
	cfg := &Config{
		MFAEnabled:       mfaEnabled,
		MFAEncryptionKey: os.Getenv("MFA_ENCRYPTION_KEY"),
		Port:             getEnv("PORT", "8080"),
		Environment:      getEnv("ENV", "development"),
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable"),
		RedisURL:         getEnv("REDIS_URL", "localhost:6379"),
		UploadDir:        getEnv("UPLOAD_DIR", "uploads"),
		AppURL:           getEnv("APP_URL", "http://localhost:3000"),
	}
	loadSMTP(cfg)
	loadOutbox(cfg)
	return cfg
}

func loadOutbox(cfg *Config) {
	c := OutboxConfig{
		Publisher:     strings.ToLower(strings.TrimSpace(getEnv("OUTBOX_PUBLISHER", "log"))),
		WebhookURL:    strings.TrimSpace(os.Getenv("OUTBOX_WEBHOOK_URL")),
		WebhookSecret: os.Getenv("OUTBOX_WEBHOOK_SECRET"),
		BatchSize:     envInt("OUTBOX_BATCH_SIZE", 50, 1, 500),
		MaxRetries:    envInt("OUTBOX_MAX_RETRIES", 5, 1, 20),
		Interval:      time.Duration(envInt("OUTBOX_INTERVAL_SECONDS", 1, 1, 300)) * time.Second,
	}
	switch c.Publisher {
	case "log", "webhook":
	default:
		log.Fatal("OUTBOX_PUBLISHER must be log or webhook.")
	}
	if c.Publisher == "webhook" && c.WebhookURL == "" {
		log.Fatal("OUTBOX_WEBHOOK_URL is required when OUTBOX_PUBLISHER=webhook.")
	}
	cfg.Outbox = c
}

func envInt(key string, fallback, min, max int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		log.Printf("%s must be a number between %d and %d; using %d.", key, min, max, fallback)
		return fallback
	}
	return value
}

func parseMFAEnabled(value string) (bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return true, nil
	}
	return strconv.ParseBool(value)
}

// loadSMTP reads either the SMTP_* variables or, when SMTP_HOST is unset, the
// Laravel-style MAIL_* variables. The two sets are never mixed, so a local
// SMTP_HOST (e.g. Mailpit from run.ps1) never receives MAIL_* credentials.
func loadSMTP(cfg *Config) {
	var name, address string
	if host := strings.TrimSpace(os.Getenv("SMTP_HOST")); host != "" {
		cfg.SMTPSource = "SMTP_*"
		cfg.SMTPHost = host
		cfg.SMTPPort = getEnv("SMTP_PORT", "")
		cfg.SMTPUsername = os.Getenv("SMTP_USERNAME")
		cfg.SMTPPassword = os.Getenv("SMTP_PASSWORD")
		cfg.SMTPEncryption = os.Getenv("SMTP_ENCRYPTION")
		if from := strings.TrimSpace(os.Getenv("SMTP_FROM")); from != "" {
			address = from // may already be "Name <addr>"
		}
	} else if host := strings.TrimSpace(os.Getenv("MAIL_HOST")); host != "" {
		cfg.SMTPSource = "MAIL_*"
		cfg.SMTPHost = host
		cfg.SMTPPort = os.Getenv("MAIL_PORT")
		cfg.SMTPUsername = os.Getenv("MAIL_USERNAME")
		cfg.SMTPPassword = os.Getenv("MAIL_PASSWORD")
		cfg.SMTPEncryption = os.Getenv("MAIL_ENCRYPTION")
		address = strings.TrimSpace(os.Getenv("MAIL_FROM_ADDRESS"))
		name = strings.TrimSpace(os.Getenv("MAIL_FROM_NAME"))
	} else {
		return
	}
	cfg.SMTPPort = strings.TrimSpace(cfg.SMTPPort)
	cfg.SMTPEncryption = normalizeEncryption(cfg.SMTPEncryption, cfg.SMTPHost, cfg.SMTPPort)
	if cfg.SMTPPort == "" {
		cfg.SMTPPort = map[string]string{"tls": "465", "starttls": "587", "none": "1025"}[cfg.SMTPEncryption]
	}
	// Providers such as Microsoft 365 only accept mail from the signed-in mailbox,
	// so default the sender to the username when it is an address.
	if address == "" && strings.Contains(cfg.SMTPUsername, "@") {
		address = cfg.SMTPUsername
	}
	if address == "" {
		address = "no-reply@paycore.local"
	}
	if name == "" || strings.Contains(name, "${") {
		name = "PayCore"
	}
	if parsed, err := mail.ParseAddress(address); err == nil && strings.Contains(address, "<") {
		cfg.SMTPFrom = parsed.String()
	} else {
		cfg.SMTPFrom = (&mail.Address{Name: name, Address: address}).String()
	}
}

// normalizeEncryption maps "ssl"/"smtps" to "tls" and "" to a default from the port and host.
func normalizeEncryption(value, host, port string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tls", "ssl", "smtps":
		// Laravel's MAIL_ENCRYPTION=tls means STARTTLS on 587; only 465 is implicit TLS.
		if port == "465" || strings.EqualFold(value, "ssl") || strings.EqualFold(value, "smtps") {
			return "tls"
		}
		return "starttls"
	case "starttls":
		return "starttls"
	case "none", "null", "false", "off":
		return "none"
	case "":
		if port == "465" {
			return "tls"
		}
		if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return "none"
		}
		return "starttls"
	default:
		return strings.ToLower(strings.TrimSpace(value)) // rejected by mailer.SMTP.Validate
	}
}

func getEnv(key, fallback string) string {
	if val, exists := os.LookupEnv(key); exists {
		return val
	}
	return fallback
}
