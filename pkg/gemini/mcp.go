package gemini

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/genai"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
)

// MCPExecutor executes an MCP tool by name.
type MCPExecutor func(ctx context.Context, name string, args any) (string, error)

// ConnectMCPServers connects to the specified endpoints using the official MCP Go SDK,
// returning GenAI tool declarations, an executor for function calls, and a cleanup function.
func ConnectMCPServers(ctx context.Context, servers []geminiv1alpha1.MCPServerSpec) ([]*genai.Tool, MCPExecutor, func()) {
	if len(servers) == 0 {
		return nil, func(ctx context.Context, name string, args any) (string, error) {
			return "", fmt.Errorf("no MCP servers configured")
		}, func() {}
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "geminiset-operator",
		Version: "v0.1.0",
	}, nil)

	httpClient := &http.Client{Timeout: 15 * time.Second}
	sessions := make([]*mcp.ClientSession, 0, len(servers))
	toolMap := make(map[string]*mcp.ClientSession)
	var funcDecls []*genai.FunctionDeclaration

	for _, s := range servers {
		var transport mcp.Transport
		if strings.Contains(s.Endpoint, "/sse") {
			transport = &mcp.SSEClientTransport{
				Endpoint:   s.Endpoint,
				HTTPClient: httpClient,
			}
		} else {
			transport = &mcp.StreamableClientTransport{
				Endpoint:             s.Endpoint,
				HTTPClient:           httpClient,
				DisableStandaloneSSE: true,
			}
		}

		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			log.Printf("[GeminiMCP] Warning: failed to connect to MCP server %s (%s): %v", s.Name, s.Endpoint, err)
			continue
		}
		sessions = append(sessions, session)

		res, err := session.ListTools(ctx, nil)
		if err != nil {
			log.Printf("[GeminiMCP] Warning: failed to list tools from MCP server %s: %v", s.Name, err)
			continue
		}

		log.Printf("[GeminiMCP] Discovered %d tools from MCP server %s", len(res.Tools), s.Name)
		for _, t := range res.Tools {
			toolMap[t.Name] = session
			decl := &genai.FunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
			}
			if t.InputSchema != nil {
				decl.ParametersJsonSchema = t.InputSchema
			}
			funcDecls = append(funcDecls, decl)
		}
	}

	var tools []*genai.Tool
	if len(funcDecls) > 0 {
		tools = []*genai.Tool{
			{
				FunctionDeclarations: funcDecls,
			},
		}
	}

	executor := func(ctx context.Context, name string, args any) (string, error) {
		session, ok := toolMap[name]
		if !ok {
			return "", fmt.Errorf("no MCP server found for tool %q", name)
		}

		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		})
		if err != nil {
			return "", fmt.Errorf("MCP tool call %q failed: %w", name, err)
		}
		if res.IsError {
			return "", fmt.Errorf("MCP tool error: %v", res.Content)
		}

		var sb strings.Builder
		for _, content := range res.Content {
			if tc, ok := content.(*mcp.TextContent); ok {
				sb.WriteString(tc.Text)
			} else if str, ok := content.(fmt.Stringer); ok {
				sb.WriteString(str.String())
			} else {
				sb.WriteString(fmt.Sprintf("%v", content))
			}
		}
		if sb.Len() == 0 {
			return fmt.Sprintf("%v", res.StructuredContent), nil
		}
		return sb.String(), nil
	}

	cleanup := func() {
		for _, s := range sessions {
			_ = s.Close()
		}
	}

	return tools, executor, cleanup
}
