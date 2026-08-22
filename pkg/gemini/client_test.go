package gemini

import (
	"testing"
)

func TestNewClient_MissingAPIKey(t *testing.T) {
	// Verify NewClient returns error when API key is not provided or in env
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	client, err := NewClient("", "gemini-3.7-flash")
	if err == nil {
		t.Errorf("expected error from NewClient when GEMINI_API_KEY is empty, got client %v", client)
	}
}
