package config

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"unicode"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const (
	keyMailMode            = "ANPFUEL_MAIL_MODE"
	keyMailMaxCodesPerHour = "ANPFUEL_MAIL_MAX_CODES_PER_HOUR"
	keySMTPHost            = "ANPFUEL_SMTP_HOST"
	keySMTPPort            = "ANPFUEL_SMTP_PORT"
	keySMTPTLS             = "ANPFUEL_SMTP_TLS"
	keySMTPUsername        = "ANPFUEL_SMTP_USERNAME"
	keySMTPPasswordFile    = "ANPFUEL_SMTP_PASSWORD_FILE"
	keySMTPFrom            = "ANPFUEL_SMTP_FROM"
	maxSMTPSecretBytes     = 4096
)

// SMTPConfig holds startup-validated settings. It is excluded from LogValue.
type SMTPConfig struct {
	Host     string
	Port     int
	TLS      string
	Username string
	Password string
	From     string
}

func loadMail(cfg *Config, getenv func(string) (string, bool)) error {
	mode, _ := getenv(keyMailMode)
	switch mode {
	case "":
		mode = "off"
	case "off", "memory", "smtp":
	default:
		return fmt.Errorf("%s must be off, memory or smtp", keyMailMode)
	}
	cfg.MailMode = mode
	cfg.MailMaxCodesPerHour = account.MaxAggregateCodesPerHour
	if raw, _ := getenv(keyMailMaxCodesPerHour); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < account.MinAggregateCodesPerHour || value > account.MaxConfiguredAggregateCodesPerHour {
			return fmt.Errorf("%s must be %d..%d", keyMailMaxCodesPerHour, account.MinAggregateCodesPerHour, account.MaxConfiguredAggregateCodesPerHour)
		}
		cfg.MailMaxCodesPerHour = value
	}
	if mode != "smtp" {
		return nil
	}
	values := make(map[string]string)
	for _, key := range []string{keySMTPHost, keySMTPPort, keySMTPTLS, keySMTPUsername, keySMTPPasswordFile, keySMTPFrom} {
		raw, _ := getenv(key)
		if strings.TrimSpace(raw) == "" || strings.ContainsFunc(raw, unicode.IsControl) {
			return fmt.Errorf("%s must be set and contain no controls when %s is smtp", key, keyMailMode)
		}
		values[key] = raw
	}
	host := values[keySMTPHost]
	if strings.ContainsFunc(host, unicode.IsSpace) || strings.ContainsAny(host, "/@[]") || (strings.Contains(host, ":") && net.ParseIP(host) == nil) {
		return fmt.Errorf("%s must be a hostname or IP address", keySMTPHost)
	}
	port, err := strconv.Atoi(values[keySMTPPort])
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s must be a TCP port", keySMTPPort)
	}
	tlsMode := values[keySMTPTLS]
	if tlsMode != "implicit" && tlsMode != "starttls" {
		return fmt.Errorf("%s must be implicit or starttls", keySMTPTLS)
	}
	if !account.ValidEmail(values[keySMTPFrom]) {
		return fmt.Errorf("%s must be one bare email address", keySMTPFrom)
	}
	password, err := readSMTPPassword(values[keySMTPPasswordFile])
	if err != nil {
		return err
	}
	cfg.SMTP = SMTPConfig{
		Host: host, Port: port, TLS: tlsMode, Username: values[keySMTPUsername],
		Password: password, From: strings.TrimSpace(values[keySMTPFrom]),
	}
	return nil
}

func readSMTPPassword(path string) (string, error) {
	unavailable := fmt.Errorf("%s must name a readable nonempty password file", keySMTPPasswordFile)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", unavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return "", unavailable
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSMTPSecretBytes+1))
	if err != nil || len(data) > maxSMTPSecretBytes {
		return "", unavailable
	}
	secret := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if strings.TrimSpace(secret) == "" || strings.ContainsFunc(secret, unicode.IsControl) {
		return "", unavailable
	}
	return secret, nil
}
