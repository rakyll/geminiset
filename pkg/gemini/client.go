package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

// Engine defines the interface for Gemini operations.
type Engine interface {
	SynthesizeWorkload(ctx context.Context, req WorkloadSynthesisRequest) (*WorkloadSynthesisResponse, error)
	SchedulePod(ctx context.Context, req SchedulingRequest) (*SchedulingDecision, error)
	Model() string
}

// Client communicates with Google Gemini models using the official Google Gen AI SDK.
type Client struct {
	apiKey string
	model  string
	client *genai.Client
}

// NewClient initializes a Gemini client using the official Google Gen AI SDK.
func NewClient(apiKey, model string) (*Client, error) {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is required")
	}

	if model == "" {
		model = os.Getenv("GEMINI_MODEL")
	}
	if model == "" {
		model = "gemini-3.7-flash"
	}

	ctx := context.Background()
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GenAI client: %w", err)
	}

	return &Client{
		apiKey: apiKey,
		model:  model,
		client: genaiClient,
	}, nil
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) SynthesizeWorkload(ctx context.Context, req WorkloadSynthesisRequest) (*WorkloadSynthesisResponse, error) {
	if c.client == nil {
		return nil, fmt.Errorf("GEMINI_API_KEY is required to invoke %s", c.model)
	}

	sysInst := BuildSynthesisSystemInstruction()
	prompt := BuildSynthesisPrompt(req)

	tools, executeTool, closeSessions := ConnectMCPServers(ctx, req.MCPServers)
	defer closeSessions()

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.Text(sysInst)[0],
		ResponseMIMEType:  "application/json",
		Temperature:       genai.Ptr(float32(0.2)),
		Tools:             tools,
	}

	contents := genai.Text(prompt)

	for round := 0; round < 5; round++ {
		resp, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
		if err != nil {
			return nil, fmt.Errorf("gemini workload synthesis failed: %w", err)
		}

		var functionCalls []*genai.FunctionCall
		if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.FunctionCall != nil {
					functionCalls = append(functionCalls, part.FunctionCall)
				}
			}
		}

		if len(functionCalls) == 0 {
			rawText := strings.TrimSpace(resp.Text())
			rawText = strings.TrimPrefix(rawText, "```json")
			rawText = strings.TrimPrefix(rawText, "```")
			rawText = strings.TrimSuffix(rawText, "```")

			var synResp WorkloadSynthesisResponse
			if err := json.Unmarshal([]byte(rawText), &synResp); err != nil {
				return nil, fmt.Errorf("failed to parse synthesized JSON from Gemini: %w (raw response: %s)", err, rawText)
			}
			return &synResp, nil
		}

		// Append the model's function-call turn
		contents = append(contents, resp.Candidates[0].Content)

		// Execute tools and append function responses
		for _, fc := range functionCalls {
			log.Printf("[GeminiMCP] Executing tool %s with arguments %v", fc.Name, fc.Args)
			result, callErr := executeTool(ctx, fc.Name, fc.Args)
			if callErr != nil {
				result = fmt.Sprintf("Error: %v", callErr)
			}
			contents = append(contents, genai.NewContentFromFunctionResponse(fc.Name, map[string]any{"result": result}, genai.RoleUser))
		}
	}

	return nil, fmt.Errorf("exceeded maximum tool calling rounds during synthesis")
}

func (c *Client) SchedulePod(ctx context.Context, req SchedulingRequest) (*SchedulingDecision, error) {
	if c.client == nil {
		return nil, fmt.Errorf("GEMINI_API_KEY is required to invoke %s", c.model)
	}

	sysInst := BuildSchedulingSystemInstruction()
	prompt := BuildSchedulingPrompt(req)

	tools, executeTool, closeSessions := ConnectMCPServers(ctx, req.MCPServers)
	defer closeSessions()

	start := time.Now()
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.Text(sysInst)[0],
		ResponseMIMEType:  "application/json",
		Temperature:       genai.Ptr(float32(0.1)),
		Tools:             tools,
	}

	contents := genai.Text(prompt)

	for round := 0; round < 5; round++ {
		resp, err := c.client.Models.GenerateContent(ctx, c.model, contents, config)
		if err != nil {
			return nil, fmt.Errorf("gemini scheduling failed: %w", err)
		}

		var functionCalls []*genai.FunctionCall
		if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.FunctionCall != nil {
					functionCalls = append(functionCalls, part.FunctionCall)
				}
			}
		}

		if len(functionCalls) == 0 {
			rawText := strings.TrimSpace(resp.Text())
			rawText = strings.TrimPrefix(rawText, "```json")
			rawText = strings.TrimPrefix(rawText, "```")
			rawText = strings.TrimSuffix(rawText, "```")

			var decision SchedulingDecision
			if err := json.Unmarshal([]byte(rawText), &decision); err != nil {
				return nil, fmt.Errorf("failed to parse scheduling decision JSON: %w (raw response: %s)", err, rawText)
			}

			decision.ModelUsed = c.model
			decision.DurationMs = time.Since(start).Milliseconds()

			return &decision, nil
		}

		contents = append(contents, resp.Candidates[0].Content)

		for _, fc := range functionCalls {
			log.Printf("[GeminiMCP] Executing tool %s with arguments %v", fc.Name, fc.Args)
			result, callErr := executeTool(ctx, fc.Name, fc.Args)
			if callErr != nil {
				result = fmt.Sprintf("Error: %v", callErr)
			}
			contents = append(contents, genai.NewContentFromFunctionResponse(fc.Name, map[string]any{"result": result}, genai.RoleUser))
		}
	}

	return nil, fmt.Errorf("exceeded maximum tool calling rounds during scheduling")
}
