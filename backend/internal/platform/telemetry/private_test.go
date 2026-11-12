package telemetry

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestPrivateContextDiscardsFreeTextFieldsAndInheritedMetadata(t *testing.T) {
	var logs bytes.Buffer
	logger := newTestLogger(t, &logs).WithRoute("PRIVATE-BOUND-ROUTE")
	ctx := ContextWithRequest(context.Background(), "PRIVATE-CLIENT-TRACE")
	ctx = ContextWithJob(ctx, "PRIVATE-JOB-TYPE", "PRIVATE-JOB-ID")
	ctx = ContextWithPrivateRequest(ctx)
	logger.Warn(ctx, "PRIVATE-EVENT-CODE", "PRIVATE-MESSAGE", "token", "PRIVATE-TOKEN")
	logger.Request(ctx, "PRIVATE-METHOD", "/v1/orgs/{org_id}/example/read", 200, time.Millisecond)
	if strings.Contains(logs.String(), "PRIVATE-") {
		t.Fatal("private context leaked free text or inherited metadata")
	}
	for _, want := range []string{`"code":"private.event"`, `"method":"OTHER"`, `"route":"/v1/orgs/{org_id}/example/read"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatal("missing safe telemetry", want)
		}
	}
	logs.Reset()
	ctx = ContextWithServerRequest(ctx, "server-generated-trace")
	ctx = ContextWithRequest(ctx, "PRIVATE-REPLACEMENT")
	logger.Error(ctx, "http.panic", "PRIVATE-PANIC")
	if strings.Contains(logs.String(), "PRIVATE-") || !strings.Contains(logs.String(), "server-generated-trace") {
		t.Fatal("private correlation did not retain only the server trace")
	}
}
