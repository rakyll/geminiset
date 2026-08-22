package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	GroupName = "geminiset.io"
	Version   = "v1alpha1"

	// SchedulerName is the custom scheduler name assigned to pods managed by GeminiSet.
	SchedulerName = "gemini-scheduler"

	// Annotation keys used by Gemini scheduler & controller
	AnnotationDecisionRationale = "geminiset.io/decision-rationale"
	AnnotationNodeScores        = "geminiset.io/node-scores"
	AnnotationScheduledBy       = "geminiset.io/scheduled-by"
	AnnotationScheduledAt       = "geminiset.io/scheduled-at"
	AnnotationPlacementHint     = "geminiset.io/placement-hint"
	AnnotationSynthesizedPrompt = "geminiset.io/synthesized-prompt"
	AnnotationAlternativeNodes  = "geminiset.io/alternatives-evaluated"
	AnnotationMCPServers        = "geminiset.io/mcp-servers"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme
)

func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&GeminiSet{},
		&GeminiSetList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="AI-Status",type="string",JSONPath=".status.aiCompilationStatus"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GeminiSet is a specialized workload controller (like a ReplicaSet) whose specifications
// can be authored in natural human languages (English, Turkish, Spanish, Japanese, German, etc.)
// and which exclusively schedules pods using the Gemini Flash AI-powered scheduler.
type GeminiSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GeminiSetSpec   `json:"spec,omitempty"`
	Status GeminiSetStatus `json:"status,omitempty"`
}

// GeminiSetSpec defines the desired state of GeminiSet.
// It supports natural language descriptions and optional explicit PodTemplate overrides.
type GeminiSetSpec struct {
	// Prompt is a freeform natural language description of the desired workload,
	// replica count, scaling behavior, placement preferences, and constraints (in any human language).
	Prompt string `json:"prompt,omitempty"`

	// Replicas is an optional explicit replica count override.
	Replicas *int32 `json:"replicas,omitempty"`

	// Constraints are optional explicit natural language guardrail rules.
	Constraints []string `json:"constraints,omitempty"`

	// Template is an optional explicit PodTemplateSpec override.
	Template *corev1.PodTemplateSpec `json:"template,omitempty"`

	// MCPServers are optional Model Context Protocol (MCP) server endpoints
	// providing external tools and live context (e.g. metrics, service catalogs, cloud pricing) to Gemini.
	MCPServers []MCPServerSpec `json:"mcpServers,omitempty"`
}

// MCPServerSpec defines an external Model Context Protocol (MCP) server endpoint.
type MCPServerSpec struct {
	// Name identifies the MCP server (e.g. "prometheus", "service-catalog").
	Name string `json:"name"`

	// Endpoint is the HTTP URL or SSE endpoint of the MCP server.
	Endpoint string `json:"endpoint"`

	// Description is an optional human-readable description of what context/tools this server provides.
	Description string `json:"description,omitempty"`
}

// GeminiSetStatus represents the current state of a GeminiSet.
type GeminiSetStatus struct {
	// Replicas is the most recently observed number of replicas.
	Replicas int32 `json:"replicas"`

	// ReadyReplicas is the number of ready pods for this GeminiSet.
	ReadyReplicas int32 `json:"readyReplicas"`

	// AvailableReplicas is the number of available pods for this GeminiSet.
	AvailableReplicas int32 `json:"availableReplicas"`

	// ObservedGeneration is the most recent generation observed for this GeminiSet.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// AICompilationStatus indicates the state of prompt synthesis ("Completed", "Synthesizing", "Failed").
	AICompilationStatus string `json:"aiCompilationStatus,omitempty"`

	// SynthesizedSummary is a human-readable explanation of how Gemini synthesized the workload.
	SynthesizedSummary string `json:"synthesizedSummary,omitempty"`

	// AIModelUsed is the Gemini model version used for compilation (e.g. "gemini-3.7-flash").
	AIModelUsed string `json:"aiModelUsed,omitempty"`

	// ReasoningDurationMs is the time spent by Gemini reasoning on this spec.
	ReasoningDurationMs int64 `json:"reasoningDurationMs,omitempty"`

	// Conditions represent the latest available observations of a GeminiSet's current state.
	Conditions []GeminiSetCondition `json:"conditions,omitempty"`
}

const (
	AICompilationStatusCompleted    = "Completed"
	AICompilationStatusSynthesizing = "Synthesizing"
	AICompilationStatusFailed       = "Failed"
)

// GeminiSetConditionType is a valid condition of a GeminiSet.
type GeminiSetConditionType string

const (
	GeminiSetConditionAvailable   GeminiSetConditionType = "Available"
	GeminiSetConditionProgressing GeminiSetConditionType = "Progressing"
	GeminiSetConditionAICompiled  GeminiSetConditionType = "AICompiled"
	GeminiSetConditionDegraded    GeminiSetConditionType = "Degraded"
)

// GeminiSetCondition describes the state of a GeminiSet at a certain point.
type GeminiSetCondition struct {
	Type               GeminiSetConditionType `json:"type"`
	Status             corev1.ConditionStatus `json:"status"`
	LastUpdateTime     metav1.Time            `json:"lastUpdateTime,omitempty"`
	LastTransitionTime metav1.Time            `json:"lastTransitionTime,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
	Message            string                 `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GeminiSetList is a collection of GeminiSets.
type GeminiSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GeminiSet `json:"items"`
}
