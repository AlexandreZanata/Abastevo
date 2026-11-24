package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const smtpTimeout = 10 * time.Second

// SMTPConfig supplies the authenticated, TLS-only mail transport.
// RootCAs defaults to the system certificate pool.
type SMTPConfig struct {
	Host     string
	Port     int
	TLS      string
	Username string
	Password string
	From     string
	RootCAs  *x509.CertPool
}

type SMTP struct {
	config  SMTPConfig
	timeout time.Duration
}

func NewSMTP(config SMTPConfig) (*SMTP, error) {
	if config.Host == "" || strings.ContainsFunc(config.Host, unicode.IsSpace) ||
		strings.ContainsFunc(config.Host, unicode.IsControl) || config.Port < 1 || config.Port > 65535 ||
		(config.TLS != "implicit" && config.TLS != "starttls") || config.Username == "" ||
		config.Password == "" || !domain.ValidEmail(config.From) || strings.ContainsRune(config.Username, 0) ||
		strings.ContainsRune(config.Password, 0) {
		return nil, domain.ErrMailDelivery
	}
	config.From = strings.TrimSpace(config.From)
	return &SMTP{config: config, timeout: smtpTimeout}, nil
}

var _ domain.LocalizedMailSender = (*SMTP)(nil)

func (s *SMTP) SendCode(ctx context.Context, address, code string) error {
	return s.SendCodeWithPurpose(ctx, address, code, domain.CodePurposeSignup, "")
}

func (s *SMTP) SendCodeWithPurpose(ctx context.Context, address, code string, purpose domain.CodePurpose, locale string) error {
	if !domain.ValidEmail(address) || len(code) != domain.CodeDigits || strings.ContainsFunc(code, func(r rune) bool { return r < '0' || r > '9' }) {
		return domain.ErrMailDelivery
	}
	address = strings.TrimSpace(address)
	message, err := s.message(address, code, purpose, locale)
	if err != nil {
		return domain.ErrMailDelivery
	}
	if err := s.deliver(ctx, address, message); err != nil {
		// SMTP replies can echo recipient, authentication or message data.
		return domain.ErrMailDelivery
	}
	return nil
}

func (s *SMTP) deliver(ctx context.Context, address, message string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.config.Host, RootCAs: s.config.RootCAs}
	var transport net.Conn = conn
	if s.config.TLS == "implicit" {
		secureConn := tls.Client(conn, tlsConfig)
		if err := secureConn.HandshakeContext(ctx); err != nil {
			return err
		}
		transport = secureConn
	}
	client, err := smtp.NewClient(transport, s.config.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.config.TLS == "starttls" {
		if advertised, _ := client.Extension("STARTTLS"); !advertised {
			return domain.ErrMailDelivery
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if state, ok := client.TLSConnectionState(); !ok || len(state.VerifiedChains) == 0 {
		return domain.ErrMailDelivery
	}
	if err := client.Auth(smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)); err != nil {
		return err
	}
	if err := client.Mail(s.config.From); err != nil {
		return err
	}
	if err := client.Rcpt(address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	// The server accepted DATA. Losing the connection during QUIT does not
	// undo delivery, so the accepted code must remain usable.
	_ = client.Quit()
	return nil
}

func (s *SMTP) message(address, code string, purpose domain.CodePurpose, locale string) (string, error) {
	subject, body := codeCopy(code, purpose, locale)
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return "", err
	}
	_, fromDomain, _ := strings.Cut(s.config.From, "@")
	var message strings.Builder
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n",
		s.config.From, address, time.Now().UTC().Format(time.RFC1123Z), hex.EncodeToString(randomID[:]), fromDomain,
		mime.QEncoding.Encode("UTF-8", subject))
	writer := quotedprintable.NewWriter(&message)
	if _, err := writer.Write([]byte(body)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return message.String(), nil
}

func codeCopy(code string, purpose domain.CodePurpose, locale string) (string, string) {
	language, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(locale)), "-")
	if language == "en" {
		action := "create your account"
		switch purpose {
		case domain.CodePurposeRecovery:
			action = "recover your account key"
		case domain.CodePurposeAttach:
			action = "add or change your recovery email"
		}
		return "Abastevo — your verification code", fmt.Sprintf(
			"Use %s to %s. This code is valid for 10 minutes.\r\n\r\nIf this wasn't you, ignore this email. We never ask for this code by phone or WhatsApp.\r\n", code, action)
	}
	action := "criar sua conta"
	switch purpose {
	case domain.CodePurposeRecovery:
		action = "recuperar a chave da sua conta"
	case domain.CodePurposeAttach:
		action = "adicionar ou alterar seu e-mail de recuperação"
	}
	return "Seu código de verificação do Abastevo", fmt.Sprintf(
		"Use %s para %s. Este código é válido por 10 minutos.\r\n\r\nSe não foi você, ignore este e-mail. Nunca pedimos este código por telefone ou WhatsApp.\r\n", code, action)
}
