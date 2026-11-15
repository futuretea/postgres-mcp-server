package http_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	internalhttp "github.com/futuretea/postgres-mcp-server/pkg/server/http"
)

func TestRequestLogUsesRouteClass(t *testing.T) {
	secret := "synthetic-token-value"
	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	handler := internalhttp.RequestMiddlewareWithLogger(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), logger)
	for _, target := range []string{
		internalhttp.MCPEndpoint + "?token=" + secret,
		internalhttp.SSEEndpoint + "/" + secret,
		internalhttp.SSEMessageEndpoint + "?session=" + secret,
		"/" + secret + "?q=" + secret,
	} {
		logs.Reset()
		req := httptest.NewRequest(http.MethodPost, target, nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
		line := logs.String()
		if strings.Contains(line, secret) || strings.Contains(line, req.URL.RequestURI()) {
			t.Fatalf("log for %s contained the request target: %s", target, line)
		}
		if !strings.Contains(line, `"route":"`) {
			t.Fatalf("log for %s missing route class: %s", target, line)
		}
	}
}
