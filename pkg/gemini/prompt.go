package gemini

import (
	"fmt"
	"strings"
)

const (
	DefaultModel = "gemini-3.7-flash"
)

// BuildSynthesisSystemInstruction creates the system prompt for compiling natural language GeminiSet specs.
func BuildSynthesisSystemInstruction() string {
	return `You are Gemini Flash 3.7 Kubernetes Workload Compiler, an AI engine integrated directly into Kubernetes.
Your job is to translate human language workload requests (written in English, Turkish, Spanish, German, French, Japanese, etc. inside the prompt)
into structured Kubernetes pod specifications, replica counts, resource requests, and placement hints.

Rules:
1. Accurately extract replica numbers even if written as words ("üç" -> 3, "cinco" -> 5, "drei" -> 3, "three" -> 3, "2").
2. Determine appropriate container images (e.g. "nginx:alpine", "redis:7-alpine", "caddy:alpine", "postgres:16-alpine", "python:3.12-slim", "node:20-alpine") based on the intent.
3. Set reasonable CPU & Memory requests/limits (e.g. 50m / 64Mi requests, 200m / 128Mi limits for lightweight services).
4. For language runtime images without a default web daemon (e.g. node, python, go), always supply a valid server command (e.g. ["node", "-e", "const http = require('http'); http.createServer((_, res) => res.end('OK')).listen(3000)"] or ["python3", "-m", "http.server", "8000"]) so the pod stays running and ready.
5. Extract placement constraints and hints directly from the natural language prompt (e.g. zone spread, node anti-affinity).
6. Provide a concise, clear intent summary and reasoning rationale.
7. Return ONLY valid JSON matching this schema:
{
  "intentSummary": "string",
  "replicas": 3,
  "containers": [
    {
      "name": "string",
      "image": "string",
      "ports": [8080],
      "cpuRequest": "50m",
      "memoryRequest": "64Mi",
      "cpuLimit": "200m",
      "memoryLimit": "128Mi",
      "command": ["executable", "arg1", "arg2"]
    }
  ],
  "placementHints": ["anti-affinity=zone-spread"],
  "reasoningRationale": "string",
  "labels": {"app": "string"},
  "annotations": {}
}`
}

// BuildSchedulingSystemInstruction creates the system prompt for scheduling pods to nodes.
func BuildSchedulingSystemInstruction() string {
	return `You are Gemini Flash 3.7 Kubernetes Scheduler, an intelligent scheduler replacing default kube-scheduler.
Your task is to analyze candidate cluster Nodes and select the single best node for the incoming Pod.

Evaluation Dimensions:
1. Resource Fit: CPU/Memory allocatable vs requested (avoid resource starvation).
2. Topology Spread & Anti-Affinity: Spread replicas across different zones/nodes to guarantee high availability; avoid placing same-owner pods on the same node.
3. Natural Language Placement Hints: Honor user constraints (e.g. "prefer zone/region", "low latency", "anti-affinity").

Output Requirement:
Return ONLY valid JSON with:
{
  "selectedNode": "node-name",
  "confidence": 0.98,
  "scoreMatrix": {
    "ResourceFit": 95,
    "TopologySpread": 90,
    "ConstraintCompliance": 90
  },
  "decisionRationale": "2-sentence explanation of why this node was chosen",
  "alternativesEvaluated": [
    {"nodeName": "other-node", "score": 75, "reason": "why it scored lower"}
  ]
}`
}

// BuildSynthesisPrompt formats the prompt for workload synthesis.
func BuildSynthesisPrompt(req WorkloadSynthesisRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Synthesize a Kubernetes workload for GeminiSet '%s/%s'.\n", req.Namespace, req.Name))
	if req.Prompt != "" {
		sb.WriteString(fmt.Sprintf("Prompt: %s\n", req.Prompt))
	}
	if req.Replicas != nil {
		sb.WriteString(fmt.Sprintf("Explicit Replicas: %d\n", *req.Replicas))
	}
	if len(req.Constraints) > 0 {
		sb.WriteString("Constraints:\n")
		for _, c := range req.Constraints {
			sb.WriteString(fmt.Sprintf("  - %s\n", c))
		}
	}
	sb.WriteString("\nReturn the JSON response adhering to the schema.")
	return sb.String()
}

// BuildSchedulingPrompt formats the prompt for pod scheduling.
func BuildSchedulingPrompt(req SchedulingRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Schedule unscheduled Pod '%s/%s' (Owner: %s):\n", req.PodNamespace, req.PodName, req.OwnerGeminiSet))
	if req.RequestedCPU != "" {
		sb.WriteString(fmt.Sprintf("Requested CPU: %s\n", req.RequestedCPU))
	}
	if req.RequestedMemory != "" {
		sb.WriteString(fmt.Sprintf("Requested Memory: %s\n", req.RequestedMemory))
	}
	if req.PlacementHint != "" {
		sb.WriteString(fmt.Sprintf("Placement Hint: %s\n", req.PlacementHint))
	}

	sb.WriteString("\nCandidate Nodes:\n")
	for _, n := range req.CandidateNodes {
		ownerPods := 0
		if req.OwnerGeminiSet != "" {
			for _, p := range n.ExistingPods {
				if strings.Contains(p, req.OwnerGeminiSet) {
					ownerPods++
				}
			}
		}
		sb.WriteString(fmt.Sprintf("- Node: %s | Zone: %s | Region: %s | Free CPU: %dm | Free RAM: %dMi | Total Pods: %d | Owner Pods: %d\n",
			n.Name, n.Zone, n.Region, n.CPUMilliFree, n.MemoryMBFree, n.PodCount, ownerPods))
	}

	sb.WriteString("\nSelect the best Node and output JSON matching the scheduling schema.")
	return sb.String()
}
