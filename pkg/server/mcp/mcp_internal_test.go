package mcp

import (
	"context"
	"errors"
	"testing"

	mcpprotocol "github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/postgres-mcp-server/pkg/toolset"
)

func TestNewToolHandlerReturnsErrorResult(t *testing.T) {
	wantErr := errors.New("boom")
	handler := newToolHandler(toolset.ServerTool{
		Handler: func(_ context.Context, params map[string]any) (string, error) {
			if params == nil {
				t.Fatal("handler params must be a non-nil map")
			}
			return "", wantErr
		},
	})

	result, err := handler(t.Context(), mcpprotocol.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler transport error = %v, want nil", err)
	}
	if !result.IsError {
		t.Fatal("handler result IsError = false, want true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("handler content count = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(mcpprotocol.TextContent)
	if !ok {
		t.Fatalf("handler content type = %T, want mcp.TextContent", result.Content[0])
	}
	if text.Type != "text" || text.Text != wantErr.Error() {
		t.Fatalf("handler text = {Type:%q Text:%q}, want {Type:%q Text:%q}", text.Type, text.Text, "text", wantErr)
	}
}

func TestNewToolHandlerNormalizesRawArgumentsBeforeCallingHandler(t *testing.T) {
	invalidParams := errors.New("[postgres_invalid_params] invalid parameters")
	tests := []struct {
		name       string
		raw        any
		wantCalled bool
		wantParams map[string]any
	}{
		{name: "object is preserved", raw: map[string]any{}, wantCalled: true, wantParams: map[string]any{}},
		{name: "missing arguments are rejected", raw: nil},
		{name: "array arguments are rejected", raw: []any{}},
		{name: "scalar arguments are rejected", raw: "not an object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := 0
			handler := newToolHandler(toolset.ServerTool{
				NormalizeRawArguments: func(raw any) (map[string]any, error) {
					params, ok := raw.(map[string]any)
					if !ok {
						return nil, invalidParams
					}
					return params, nil
				},
				Handler: func(_ context.Context, params map[string]any) (string, error) {
					called++
					if !tt.wantCalled {
						t.Fatalf("handler received rejected raw arguments: %#v", params)
					}
					if len(params) != len(tt.wantParams) {
						t.Fatalf("handler params = %#v, want %#v", params, tt.wantParams)
					}
					return "ok", nil
				},
			})

			result, err := handler(t.Context(), mcpprotocol.CallToolRequest{
				Params: mcpprotocol.CallToolParams{Arguments: tt.raw},
			})
			if err != nil {
				t.Fatalf("handler transport error = %v, want nil", err)
			}
			if tt.wantCalled {
				if called != 1 || result.IsError {
					t.Fatalf("valid raw arguments called=%d result=%+v, want one successful handler call", called, result)
				}
				return
			}
			if called != 0 {
				t.Fatalf("invalid raw arguments called handler %d times, want 0", called)
			}
			if !result.IsError || len(result.Content) != 1 {
				t.Fatalf("invalid raw arguments result = %+v, want one error text", result)
			}
			text, ok := result.Content[0].(mcpprotocol.TextContent)
			if !ok || text.Type != "text" || text.Text != invalidParams.Error() {
				t.Fatalf("invalid raw arguments text = %#v, want %q", result.Content[0], invalidParams)
			}
		})
	}
}
