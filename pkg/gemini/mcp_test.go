package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
)

func TestConnectMCPServers(t *testing.T) {
	// 1. Create official MCP Server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "catalog-server",
		Version: "1.0.0",
	}, nil)

	server.AddTool(&mcp.Tool{
		Name:        "get_approved_image",
		Description: "Fetches approved image digest from corporate catalog",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service": map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: "gcr.io/enterprise/service:v3.0.1@sha256:998877",
				},
			},
		}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()

	// 2. Test ConnectMCPServers helper
	servers := []geminiv1alpha1.MCPServerSpec{
		{
			Name:     "catalog-server",
			Endpoint: ts.URL,
		},
	}

	tools, executeTool, cleanup := ConnectMCPServers(ctx, servers)
	defer cleanup()

	if len(tools) != 1 || len(tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 GenAI tool declaration, got %v", tools)
	}

	if tools[0].FunctionDeclarations[0].Name != "get_approved_image" {
		t.Errorf("unexpected tool name: %s", tools[0].FunctionDeclarations[0].Name)
	}

	// 3. Execute tool call
	result, err := executeTool(ctx, "get_approved_image", map[string]any{"service": "auth"})
	if err != nil {
		t.Fatalf("failed to execute tool: %v", err)
	}
	if result != "gcr.io/enterprise/service:v3.0.1@sha256:998877" {
		t.Errorf("unexpected tool result: %s", result)
	}
}
