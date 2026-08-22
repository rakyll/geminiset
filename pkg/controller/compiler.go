package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
	"github.com/rakyll/geminiset/pkg/gemini"
)

type compiledCacheEntry struct {
	specHash  string
	template  *corev1.PodTemplateSpec
	synthesis *gemini.WorkloadSynthesisResponse
}

// WorkloadCompiler compiles high-level natural language GeminiSet specifications
// into concrete PodTemplateSpecs managed by the gemini-scheduler.
type WorkloadCompiler struct {
	engine gemini.Engine
	cache  map[string]compiledCacheEntry
	mu     sync.RWMutex
}

func NewWorkloadCompiler(engine gemini.Engine) *WorkloadCompiler {
	return &WorkloadCompiler{
		engine: engine,
		cache:  make(map[string]compiledCacheEntry),
	}
}

func computeSpecHash(set *geminiv1alpha1.GeminiSet) string {
	h := sha256.New()
	h.Write([]byte(set.Spec.Prompt))
	for _, c := range set.Spec.Constraints {
		h.Write([]byte(c))
	}
	for _, s := range set.Spec.MCPServers {
		h.Write([]byte(s.Name + s.Endpoint))
	}
	if set.Spec.Replicas != nil {
		h.Write([]byte(fmt.Sprintf("%d", *set.Spec.Replicas)))
	}
	if set.Spec.Template != nil {
		if tplBytes, err := json.Marshal(set.Spec.Template); err == nil {
			h.Write(tplBytes)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Compile compiles the given GeminiSet into a synthesized PodTemplateSpec and metadata.
func (c *WorkloadCompiler) Compile(ctx context.Context, set *geminiv1alpha1.GeminiSet) (*corev1.PodTemplateSpec, *gemini.WorkloadSynthesisResponse, error) {
	key := fmt.Sprintf("%s/%s", set.Namespace, set.Name)
	specHash := computeSpecHash(set)

	c.mu.RLock()
	if cached, ok := c.cache[key]; ok && cached.specHash == specHash {
		c.mu.RUnlock()
		return cached.template.DeepCopy(), cached.synthesis, nil
	}
	c.mu.RUnlock()

	if set.Spec.Template != nil && set.Spec.Prompt == "" {
		tpl := set.Spec.Template.DeepCopy()
		tpl.Spec.SchedulerName = geminiv1alpha1.SchedulerName
		if tpl.Labels == nil {
			tpl.Labels = make(map[string]string)
		}
		tpl.Labels["geminiset.io/workload"] = set.Name
		tpl.Labels["geminiset.io/geminiset"] = set.Name

		if tpl.Annotations == nil {
			tpl.Annotations = make(map[string]string)
		}
		if len(set.Spec.Constraints) > 0 {
			tpl.Annotations[geminiv1alpha1.AnnotationPlacementHint] = strings.Join(set.Spec.Constraints, "; ")
		}

		resp := &gemini.WorkloadSynthesisResponse{
			IntentSummary:      "Explicit PodTemplate with natural language scheduling constraints",
			Replicas:           getReplicasOrDefault(set.Spec.Replicas),
			ReasoningRationale: "Using explicit PodTemplateSpec with gemini-scheduler and natural language constraints.",
		}
		c.mu.Lock()
		c.cache[key] = compiledCacheEntry{specHash: specHash, template: tpl, synthesis: resp}
		c.mu.Unlock()
		return tpl, resp, nil
	}

	req := gemini.WorkloadSynthesisRequest{
		Name:         set.Name,
		Namespace:    set.Namespace,
		Prompt:       set.Spec.Prompt,
		Replicas:     set.Spec.Replicas,
		Constraints:  set.Spec.Constraints,
		BaseTemplate: set.Spec.Template,
		MCPServers:   set.Spec.MCPServers,
	}

	synthesis, err := c.engine.SynthesizeWorkload(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("gemini workload synthesis failed: %w", err)
	}

	if set.Spec.Replicas != nil {
		synthesis.Replicas = *set.Spec.Replicas
	}

	labels := map[string]string{
		"geminiset.io/workload":  set.Name,
		"geminiset.io/geminiset": set.Name,
	}
	for k, v := range synthesis.Labels {
		labels[k] = v
	}

	annotations := map[string]string{
		geminiv1alpha1.AnnotationSynthesizedPrompt: set.Spec.Prompt,
	}
	if len(synthesis.PlacementHints) > 0 {
		annotations[geminiv1alpha1.AnnotationPlacementHint] = strings.Join(synthesis.PlacementHints, "; ")
	}
	if len(set.Spec.MCPServers) > 0 {
		if mcpJSON, err := json.Marshal(set.Spec.MCPServers); err == nil {
			annotations[geminiv1alpha1.AnnotationMCPServers] = string(mcpJSON)
		}
	}
	for k, v := range synthesis.Annotations {
		annotations[k] = v
	}

	var containers []corev1.Container
	for _, cs := range synthesis.Containers {
		var ports []corev1.ContainerPort
		for _, p := range cs.Ports {
			ports = append(ports, corev1.ContainerPort{
				ContainerPort: p,
				Protocol:      corev1.ProtocolTCP,
			})
		}

		resources := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
			Limits:   corev1.ResourceList{},
		}
		if cs.CPURequest != "" {
			if q, err := resource.ParseQuantity(cs.CPURequest); err == nil {
				resources.Requests[corev1.ResourceCPU] = q
			}
		}
		if cs.MemoryRequest != "" {
			if q, err := resource.ParseQuantity(cs.MemoryRequest); err == nil {
				resources.Requests[corev1.ResourceMemory] = q
			}
		}
		if cs.CPULimit != "" {
			if q, err := resource.ParseQuantity(cs.CPULimit); err == nil {
				resources.Limits[corev1.ResourceCPU] = q
			}
		}
		if cs.MemoryLimit != "" {
			if q, err := resource.ParseQuantity(cs.MemoryLimit); err == nil {
				resources.Limits[corev1.ResourceMemory] = q
			}
		}

		var env []corev1.EnvVar
		for k, v := range cs.Environment {
			env = append(env, corev1.EnvVar{Name: k, Value: v})
		}

		var probe *corev1.Probe
		if cs.ReadinessProbe != nil && cs.ReadinessProbe.Port > 0 {
			probe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: cs.ReadinessProbe.Path,
						Port: intstr.FromInt32(cs.ReadinessProbe.Port),
					},
				},
				InitialDelaySeconds: 2,
				PeriodSeconds:       5,
			}
		}

		containers = append(containers, corev1.Container{
			Name:           cs.Name,
			Image:          cs.Image,
			Ports:          ports,
			Resources:      resources,
			Env:            env,
			Command:        cs.Command,
			ReadinessProbe: probe,
		})
	}

	if len(containers) == 0 {
		containers = append(containers, corev1.Container{
			Name:  "web-server",
			Image: "nginx:1.27-alpine",
			Ports: []corev1.ContainerPort{{ContainerPort: 80}},
		})
	}

	podTemplate := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			SchedulerName: geminiv1alpha1.SchedulerName,
			Containers:    containers,
			RestartPolicy: corev1.RestartPolicyAlways,
		},
	}

	c.mu.Lock()
	c.cache[key] = compiledCacheEntry{
		specHash:  specHash,
		template:  podTemplate.DeepCopy(),
		synthesis: synthesis,
	}
	c.mu.Unlock()

	return podTemplate, synthesis, nil
}

func getReplicasOrDefault(rep *int32) int32 {
	if rep != nil {
		return *rep
	}
	return 3
}

var _ = apierrors.IsNotFound
