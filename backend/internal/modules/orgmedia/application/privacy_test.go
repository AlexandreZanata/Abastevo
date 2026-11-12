package application_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
)

func TestPrivateResultsNeverUseSensitiveDefaultFormatting(t *testing.T) {
	for _, value := range []any{application.Access{Token: "SYNTHETIC-ACCESS-CREDENTIAL"}, application.Content{Bytes: []byte("SYNTHETIC-DOCUMENT-FRAGMENT"), MIME: "image/jpeg"}} {
		var log bytes.Buffer
		slog.New(slog.NewJSONHandler(&log, nil)).Info("private transfer", "result", value)
		if strings.Contains(log.String(), "SYNTHETIC") || strings.Contains(log.String(), "capability") || strings.Contains(log.String(), "Bytes") {
			t.Fatal("structured logging exposed a private result")
		}
		for _, format := range []string{"%v", "%+v", "%#v"} {
			if strings.Contains(fmt.Sprintf(format, value), "SYNTHETIC") {
				t.Fatal("default formatting exposed a private result")
			}
		}
	}
}
