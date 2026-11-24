//go:build integration

package adapters

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	accountmail "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/mail"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func TestPGSMTPRefusalsCompensateIssuedCodeAndKeepLogsRedacted(t *testing.T) {
	fixture := freshGoogleService(t)
	svc, clock := fixture.svc, fixture.clock
	store := svc.Store.(*PGStore)
	const (
		address    = "mail-rollback@example.invalid"
		code       = "482916"
		smtpSecret = "synthetic-no-log-password"
	)
	svc.CodeGen = func() (string, error) { return code, nil }
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	svc.OnCodeDeliveryFailure = func(ctx context.Context, compensationFailed bool) {
		logger.WarnContext(ctx, "verification code delivery failed", "compensation_failed", compensationFailed)
	}
	for _, response := range []string{"454 temporarily refused", "554 permanently refused"} {
		t.Run(response, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
				in := bufio.NewReader(conn)
				_, _ = fmt.Fprint(conn, "220 smtp.example.invalid ready\r\n")
				if line, err := in.ReadString('\n'); err != nil || !strings.HasPrefix(line, "EHLO ") {
					return
				}
				_, _ = fmt.Fprint(conn, "250-smtp.example.invalid\r\n250 STARTTLS\r\n")
				if line, err := in.ReadString('\n'); err != nil || line != "STARTTLS\r\n" {
					return
				}
				_, _ = fmt.Fprintf(conn, "%s %s %s %s\r\n", response, address, code, smtpSecret)
			}()
			host, portText, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				t.Fatal(err)
			}
			svc.Mail, err = accountmail.NewSMTP(accountmail.SMTPConfig{
				Host: host, Port: port, TLS: "starttls", Username: "synthetic-smtp-user", Password: smtpSecret, From: "sender@example.invalid",
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.RequestCodeWithPurpose(t.Context(), address, domain.CodePurposeRecovery, "en"); !errors.Is(err, domain.ErrMailDelivery) {
				t.Fatalf("expected redacted SMTP refusal, got %v", err)
			}
			if n, err := store.CountAllCodesSince(t.Context(), clock.NowUnix()-3600); n != 0 || err != nil {
				t.Fatalf("SMTP refusal retained %d issuances: %v", n, err)
			}
		})
	}
	if logs.Len() == 0 || strings.Contains(logs.String(), `"compensation_failed":true`) {
		t.Fatal("expected successful rollback diagnostic")
	}
	for _, private := range []string{address, code, smtpSecret} {
		if strings.Contains(logs.String(), private) {
			t.Fatal("SMTP failure diagnostic leaked account data")
		}
	}
}
