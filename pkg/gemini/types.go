package gemini

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
)

// WorkloadSynthesisRequest represents input to Gemini for compiling a natural language GeminiSet spec.
type WorkloadSynthesisRequest struct {
	Name         string                         `json:"name"`
	Namespace    string                         `json:"namespace"`
	Prompt       string                         `json:"prompt,omitempty"`
	Replicas     *int32                         `json:"replicas,omitempty"`
	Constraints  []string                       `json:"constraints,omitempty"`
	BaseTemplate *corev1.PodTemplateSpec        `json:"baseTemplate,omitempty"`
	MCPServers   []geminiv1alpha1.MCPServerSpec `json:"mcpServers,omitempty"`
}

// WorkloadSynthesisResponse is the structured output synthesized by Gemini Flash 3.7.
type WorkloadSynthesisResponse struct {
	IntentSummary      string                 `json:"intentSummary"`
	Replicas           int32                  `json:"replicas"`
	Containers         []ContainerSynthesis   `json:"containers"`
	PlacementHints     []string               `json:"placementHints"`
	ReasoningRationale string                 `json:"reasoningRationale"`
	Labels             map[string]string      `json:"labels"`
	Annotations        map[string]string      `json:"annotations"`
	PodSecurityContext map[string]interface{} `json:"podSecurityContext,omitempty"`
}

func (w *WorkloadSynthesisResponse) UnmarshalJSON(data []byte) error {
	type Alias WorkloadSynthesisResponse
	aux := struct {
		PlacementHintsRaw json.RawMessage `json:"placementHints"`
		*Alias
	}{
		Alias: (*Alias)(w),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Flexibly parse placementHints as either []string or object
	if len(aux.PlacementHintsRaw) > 0 {
		var strList []string
		if err := json.Unmarshal(aux.PlacementHintsRaw, &strList); err == nil {
			w.PlacementHints = strList
		} else {
			// If it's an object, format it as string
			var rawMap map[string]interface{}
			if err := json.Unmarshal(aux.PlacementHintsRaw, &rawMap); err == nil {
				for k, v := range rawMap {
					w.PlacementHints = append(w.PlacementHints, fmt.Sprintf("%s=%v", k, v))
				}
			}
		}
	}
	return nil
}

// ContainerSynthesis defines synthesized container specifications.
type ContainerSynthesis struct {
	Name           string            `json:"name"`
	Image          string            `json:"image"`
	Ports          []int32           `json:"ports,omitempty"`
	CPURequest     string            `json:"cpuRequest,omitempty"`
	MemoryRequest  string            `json:"memoryRequest,omitempty"`
	CPULimit       string            `json:"cpuLimit,omitempty"`
	MemoryLimit    string            `json:"memoryLimit,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"`
	Command        []string          `json:"command,omitempty"`
	ReadinessProbe *ProbeSynthesis   `json:"readinessProbe,omitempty"`
}

func (c *ContainerSynthesis) UnmarshalJSON(data []byte) error {
	type PortObj struct {
		ContainerPort int32 `json:"containerPort"`
		Port          int32 `json:"port"`
	}
	type ResourceBlock struct {
		Requests map[string]string `json:"requests"`
		Limits   map[string]string `json:"limits"`
	}

	aux := struct {
		Name           string            `json:"name"`
		Image          string            `json:"image"`
		PortsRaw       json.RawMessage   `json:"ports"`
		CPURequest     string            `json:"cpuRequest"`
		MemoryRequest  string            `json:"memoryRequest"`
		CPULimit       string            `json:"cpuLimit"`
		MemoryLimit    string            `json:"memoryLimit"`
		Resources      *ResourceBlock    `json:"resources"`
		Environment    map[string]string `json:"environment"`
		Command        []string          `json:"command"`
		ReadinessProbe *ProbeSynthesis   `json:"readinessProbe"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	c.Name = aux.Name
	c.Image = aux.Image
	c.CPURequest = aux.CPURequest
	c.MemoryRequest = aux.MemoryRequest
	c.CPULimit = aux.CPULimit
	c.MemoryLimit = aux.MemoryLimit
	c.Environment = aux.Environment
	c.Command = aux.Command
	c.ReadinessProbe = aux.ReadinessProbe

	// If resources block is used
	if aux.Resources != nil {
		if c.CPURequest == "" && aux.Resources.Requests != nil {
			c.CPURequest = aux.Resources.Requests["cpu"]
		}
		if c.MemoryRequest == "" && aux.Resources.Requests != nil {
			c.MemoryRequest = aux.Resources.Requests["memory"]
		}
		if c.CPULimit == "" && aux.Resources.Limits != nil {
			c.CPULimit = aux.Resources.Limits["cpu"]
		}
		if c.MemoryLimit == "" && aux.Resources.Limits != nil {
			c.MemoryLimit = aux.Resources.Limits["memory"]
		}
	}

	// Flexibly parse ports: [8080] or [{"containerPort": 8080}]
	if len(aux.PortsRaw) > 0 {
		var intPorts []int32
		if err := json.Unmarshal(aux.PortsRaw, &intPorts); err == nil {
			c.Ports = intPorts
		} else {
			var objPorts []PortObj
			if err := json.Unmarshal(aux.PortsRaw, &objPorts); err == nil {
				for _, p := range objPorts {
					if p.ContainerPort > 0 {
						c.Ports = append(c.Ports, p.ContainerPort)
					} else if p.Port > 0 {
						c.Ports = append(c.Ports, p.Port)
					}
				}
			}
		}
	}

	return nil
}

// ProbeSynthesis defines a synthesized readiness probe.
type ProbeSynthesis struct {
	Path string `json:"path,omitempty"`
	Port int32  `json:"port,omitempty"`
}

// NodeTelemetry represents the observed state and topology of a cluster Node.
type NodeTelemetry struct {
	Name              string            `json:"name"`
	Zone              string            `json:"zone"`
	Region            string            `json:"region"`
	InstanceType      string            `json:"instanceType"`
	CPUCapacity       string            `json:"cpuCapacity"`
	CPUAllocatable    string            `json:"cpuAllocatable"`
	MemoryCapacity    string            `json:"memoryCapacity"`
	MemoryAllocatable string            `json:"memoryAllocatable"`
	CPUMilliFree      int64             `json:"cpuMilliFree"`
	MemoryMBFree      int64             `json:"memoryMBFree"`
	ExistingPods      []string          `json:"existingPods"`
	PodCount          int               `json:"podCount"`
	Labels            map[string]string `json:"labels"`
	Taints            []string          `json:"taints,omitempty"`
}

// SchedulingRequest contains all cluster context needed for Gemini to make a scheduling decision.
type SchedulingRequest struct {
	PodName         string                         `json:"podName"`
	PodNamespace    string                         `json:"podNamespace"`
	OwnerGeminiSet  string                         `json:"ownerGeminiSet,omitempty"`
	RequestedCPU    string                         `json:"requestedCPU"`
	RequestedMemory string                         `json:"requestedMemory"`
	PlacementHint   string                         `json:"placementHint,omitempty"`
	PodAnnotations  map[string]string              `json:"podAnnotations,omitempty"`
	CandidateNodes  []NodeTelemetry                `json:"candidateNodes"`
	MCPServers      []geminiv1alpha1.MCPServerSpec `json:"mcpServers,omitempty"`
}

// AlternativeEvaluation explains why a specific node scored lower.
type AlternativeEvaluation struct {
	NodeName string `json:"nodeName"`
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
}

// SchedulingDecision is the output produced by Gemini Flash 3.7.
type SchedulingDecision struct {
	SelectedNode          string                  `json:"selectedNode"`
	Confidence            float64                 `json:"confidence"`
	ScoreMatrix           map[string]int          `json:"scoreMatrix"`
	DecisionRationale     string                  `json:"decisionRationale"`
	AlternativesEvaluated []AlternativeEvaluation `json:"alternativesEvaluated"`
	ModelUsed             string                  `json:"modelUsed"`
	DurationMs            int64                   `json:"durationMs"`
}
