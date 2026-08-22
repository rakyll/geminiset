package testutil

import (
	"context"

	"github.com/rakyll/geminiset/pkg/gemini"
)

// MockEngine implements gemini.Engine for fast, hermetic unit tests.
type MockEngine struct {
	model string
}

func NewMockEngine() *MockEngine {
	return &MockEngine{
		model: "gemini-3.7-flash (mock)",
	}
}

func (m *MockEngine) Model() string {
	return m.model
}

func (m *MockEngine) SynthesizeWorkload(ctx context.Context, req gemini.WorkloadSynthesisRequest) (*gemini.WorkloadSynthesisResponse, error) {
	replicas := int32(3)
	if req.Replicas != nil && *req.Replicas > 0 {
		replicas = *req.Replicas
	}

	return &gemini.WorkloadSynthesisResponse{
		IntentSummary:      "Mock synthesized workload: 3 replicas of redis:7.2-alpine",
		Replicas:           replicas,
		PlacementHints:     []string{"anti-affinity=zone-spread"},
		ReasoningRationale: "Synthesized workload for testing",
		Containers: []gemini.ContainerSynthesis{
			{
				Name:          "redis",
				Image:         "redis:7.2-alpine",
				Ports:         []int32{6379},
				CPURequest:    "50m",
				MemoryRequest: "64Mi",
				CPULimit:      "200m",
				MemoryLimit:   "128Mi",
			},
		},
		Labels: map[string]string{
			"geminiset.io/managed-by": "gemini-controller",
			"geminiset.io/workload":   req.Name,
		},
		Annotations: map[string]string{
			"geminiset.io/synthesized-prompt": req.Prompt,
		},
	}, nil
}

func (m *MockEngine) SchedulePod(ctx context.Context, req gemini.SchedulingRequest) (*gemini.SchedulingDecision, error) {
	selected := "node-eu-west3-a"
	if len(req.CandidateNodes) > 0 {
		selected = req.CandidateNodes[0].Name
		for _, n := range req.CandidateNodes {
			if n.Zone == "europe-west3-a" || n.Region == "europe-west3" {
				selected = n.Name
				break
			}
		}
	}

	return &gemini.SchedulingDecision{
		SelectedNode:      selected,
		Confidence:        0.98,
		DecisionRationale: "Selected node with highest availability in target zone",
		ScoreMatrix: map[string]int{
			"ResourceFit":    95,
			"TopologySpread": 90,
		},
		ModelUsed:  m.model,
		DurationMs: 8,
	}, nil
}
