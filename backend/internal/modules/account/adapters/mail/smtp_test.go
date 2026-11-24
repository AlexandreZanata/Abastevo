package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const (
	testFrom     = "alerts@example.invalid"
	testTo       = "member@example.invalid"
	testCode     = "384012"
	testUser     = "test-smtp-user"
	testPassword = "synthetic-mail-password"
)

type smtpFixture struct {
	listener  net.Listener
	cert      *tls.Config
	roots     *x509.CertPool
	connected atomic.Int32
	commands  chan string
	messages  chan string
}

func newSMTPFixture(t *testing.T, implicit, offerTLS, stall bool, failAt string) *smtpFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	fixture := &smtpFixture{
		listener: listener, roots: roots,
		cert:     &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}},
		commands: make(chan string, 20), messages: make(chan string, 1),
	}
	go fixture.serve(implicit, offerTLS, stall, failAt)
	return fixture
}

func (f *smtpFixture) serve(implicit, offerTLS, stall bool, failAt string) {
	conn, err := f.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	f.connected.Add(1)
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	if stall {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	encrypted := implicit
	if implicit {
		conn = tls.Server(conn, f.cert)
	}
	reader := bufio.NewReader(conn)
	_, _ = fmt.Fprint(conn, "220 smtp.example.invalid ESMTP ready\r\n")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		f.commands <- line
		switch command, _, _ := strings.Cut(line, " "); command {
		case "EHLO":
			_, _ = fmt.Fprint(conn, "250-smtp.example.invalid\r\n")
			if offerTLS && !encrypted {
				_, _ = fmt.Fprint(conn, "250-STARTTLS\r\n")
			}
			_, _ = fmt.Fprint(conn, "250 AUTH PLAIN\r\n")
		case "STARTTLS":
			_, _ = fmt.Fprint(conn, "220 go ahead\r\n")
			conn = tls.Server(conn, f.cert)
			reader = bufio.NewReader(conn)
			encrypted = true
		case "AUTH":
			if !encrypted {
				return
			}
			_, _ = fmt.Fprint(conn, "235 authenticated\r\n")
		case "MAIL", "RCPT":
			if command == failAt {
				_, _ = fmt.Fprintf(conn, "450 refused %s %s %s\r\n", testTo, testCode, testPassword)
				return
			}
			_, _ = fmt.Fprint(conn, "250 accepted\r\n")
		case "DATA":
			_, _ = fmt.Fprint(conn, "354 send data\r\n")
			message, err := textproto.NewReader(reader).ReadDotBytes()
			if err != nil {
				return
			}
			f.messages <- string(message)
			if failAt == "DATA" {
				_, _ = fmt.Fprintf(conn, "554 refused %s %s %s\r\n", testTo, testCode, testPassword)
				return
			}
			_, _ = fmt.Fprint(conn, "250 accepted\r\n")
		case "QUIT":
			_, _ = fmt.Fprint(conn, "221 goodbye\r\n")
			return
		}
	}
}

func (f *smtpFixture) config(t *testing.T, tlsMode string) SMTPConfig {
	t.Helper()
	host, portText, err := net.SplitHostPort(f.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return SMTPConfig{Host: host, Port: port, TLS: tlsMode, Username: testUser, Password: testPassword, From: testFrom, RootCAs: f.roots}
}

func TestSMTPDeliversSingleMIMECopyOverVerifiedTLS(t *testing.T) {
	for _, tt := range []struct {
		tlsMode, locale string
		purpose         domain.CodePurpose
		snippet, advice string
	}{
		{"implicit", "pt-BR", domain.CodePurposeSignup, "criar sua conta", "Se não foi você"},
		{"starttls", "en-US", domain.CodePurposeRecovery, "recover your account key", "If this wasn't you"},
		{"implicit", "fr", domain.CodePurposeAttach, "adicionar ou alterar", "Se não foi você"},
	} {
		t.Run(tt.tlsMode+"/"+tt.locale, func(t *testing.T) {
			fixture := newSMTPFixture(t, tt.tlsMode == "implicit", true, false, "")
			sender, err := NewSMTP(fixture.config(t, tt.tlsMode))
			if err != nil {
				t.Fatal(err)
			}
			if err := sender.SendCodeWithPurpose(t.Context(), testTo, testCode, tt.purpose, tt.locale); err != nil {
				t.Fatal(err)
			}
			msg, err := mail.ReadMessage(strings.NewReader(<-fixture.messages))
			if err != nil {
				t.Fatal(err)
			}
			if msg.Header.Get("From") != testFrom || msg.Header.Get("To") != testTo || msg.Header.Get("MIME-Version") != "1.0" ||
				msg.Header.Get("Content-Type") != "text/plain; charset=UTF-8" || msg.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
				t.Fatalf("unexpected MIME headers: %v", msg.Header)
			}
			if _, err := msg.Header.Date(); err != nil || !strings.Contains(msg.Header.Get("Message-ID"), "@example.invalid>") {
				t.Fatalf("missing date or message ID: %v", msg.Header)
			}
			subject := msg.Header.Get("Subject")
			decoded, err := new(mime.WordDecoder).DecodeHeader(subject)
			if err != nil || decoded == "" || !strings.HasPrefix(subject, "=?UTF-8?q?") || strings.Contains(subject, testCode) || strings.Contains(decoded, testCode) {
				t.Fatalf("unexpected subject: %q", subject)
			}
			body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{testCode, tt.snippet, "10 minut", tt.advice, "WhatsApp"} {
				if !strings.Contains(string(body), want) {
					t.Errorf("body missing %q: %s", want, body)
				}
			}
			if strings.Contains(string(body), "://") {
				t.Error("code email must contain no links")
			}
			var recipients int
			var auth bool
			for len(fixture.commands) > 0 {
				command := <-fixture.commands
				if strings.HasPrefix(command, "RCPT TO:") {
					recipients++
					if command != "RCPT TO:<"+testTo+">" {
						t.Errorf("unexpected envelope recipient: %s", command)
					}
				}
				if command == "AUTH PLAIN "+base64.StdEncoding.EncodeToString([]byte("\x00"+testUser+"\x00"+testPassword)) {
					auth = true
				}
			}
			if recipients != 1 || !auth {
				t.Fatalf("recipient count = %d, protected auth = %t", recipients, auth)
			}
		})
	}
}

func TestSMTPRefusesPlaintextUntrustedCertificateAndHeaderControls(t *testing.T) {
	for _, tt := range []struct {
		name              string
		offerTLS, trusted bool
		address           string
		connections       int32
	}{
		{"plaintext", false, true, testTo, 1},
		{"untrusted", true, false, testTo, 1},
		{"header injection", true, true, testTo + "\r\nBcc: other@example.invalid", 0},
		{"trailing newline", true, true, testTo + "\n", 0},
		{"tab", true, true, "a\tb@example.invalid", 0},
		{"unicode control", true, true, "a\u0085b@example.invalid", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSMTPFixture(t, false, tt.offerTLS, false, "")
			cfg := fixture.config(t, "starttls")
			if !tt.trusted {
				cfg.RootCAs = x509.NewCertPool()
			}
			sender, err := NewSMTP(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := sender.SendCode(t.Context(), tt.address, testCode); !errors.Is(err, domain.ErrMailDelivery) {
				t.Fatalf("want redacted refusal, got %v", err)
			}
			if fixture.connected.Load() != tt.connections {
				t.Fatalf("want %d connections, got %d", tt.connections, fixture.connected.Load())
			}
			for len(fixture.commands) > 0 {
				if line := <-fixture.commands; strings.HasPrefix(line, "AUTH") || strings.HasPrefix(line, "MAIL") {
					t.Fatalf("SMTP accepted an unsafe connection: %s", line)
				}
			}
		})
	}
}

func TestSMTPDeadlineCancellationAndFailureAreRedacted(t *testing.T) {
	for _, failAt := range []string{"RCPT", "DATA", "stalled context", "stalled exchange"} {
		t.Run(failAt, func(t *testing.T) {
			stall := strings.HasPrefix(failAt, "stalled")
			fixture := newSMTPFixture(t, false, true, stall, failAt)
			sender, err := NewSMTP(fixture.config(t, "starttls"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			if failAt == "stalled context" {
				limited, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
				ctx = limited
			}
			if failAt == "stalled exchange" {
				sender.timeout = 30 * time.Millisecond
			}
			started := time.Now()
			err = sender.SendCode(ctx, testTo, testCode)
			if !errors.Is(err, domain.ErrMailDelivery) || time.Since(started) > time.Second {
				t.Fatalf("bounded redacted failure expected, got %v in %v", err, time.Since(started))
			}
			for _, private := range []string{testTo, testCode, testUser, testPassword} {
				if strings.Contains(err.Error(), private) {
					t.Fatal("error contains message or SMTP credentials")
				}
			}
		})
	}
}
