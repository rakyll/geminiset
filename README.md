# GeminiSet

A workload controller and scheduler for Kubernetes powered by **Google Gemini models**.

> **Disclaimer**: This is an experimental research project exploring how large language models (such as Google Gemini) can be integrated directly into the Kubernetes control plane for natural language workload declaration and scheduling. It is intended solely for research, experimentation, and demonstration purposes, and is not designed or supported for production environments.

```
   ____                _       _ ____       _   
  / ___| ___ _ __ ___ (_)_ __ (_) ___|  ___| |_ 
 | |  _ / _ \ '_ ' _ \| | '_ \| \___ \ / _ \ __|
 | |_| |  __/ | | | | | | | | | |___) |  __/ |_ 
  \____|\___|_| |_| |_|_|_| |_|_|____/ \___|\__|
  Gemini-Native Kubernetes Workloads & Scheduling
```

## Highlights

- **GeminiSet Controller**: Author workloads using natural language prompts in any language, with optional explicit overrides (`replicas`, `template`).
- **Gemini Scheduler**: Evaluates real-time node capacity, topology spread, and natural language constraints (*e.g. "strict zone anti-affinity"*, *"nodes with at least 4GiB free memory"*) to place pods and explain decisions.
- **`kubectl-geminiset` CLI**: Deploy natural language workloads from the terminal and inspect scheduling rationales with `kubectl geminiset why <pod>`.

## Overview

```
                       ┌───────────────────────────────┐
                       │       Gemini Flash 3.7        │
                       └──────▲─────────────────▲──────┘
                              │ 1. Synthesize   │ 3. Score
                              │    Spec         │    Nodes
                              ▼                 ▼
 ┌──────────────┐     ┌──────────────┐   ┌──────────────┐     ┌──────────────┐
 │    Client    ├────►│  Controller  ├──►│  Scheduler   ├────►│ Cluster Node │
 └──────────────┘     └──────────────┘   └──────────────┘     └──────────────┘
```

## 🚀 Quickstart

```bash
go install github.com/rakyll/geminiset/cmd/kubectl-geminiset@latest

# Apply an example manifest
kubectl apply -f examples/01-nodejs-hello-world.yaml

# Or create directly from the CLI
kubectl geminiset create "Deploy 4 Nginx web servers" \
  -c "strict zone anti-affinity" \
  -c "limit memory to 128Mi per pod"
```

## Examples

All manifests express container requirements, replica counts, and cluster placement rules directly in natural language:

### Node.js Hello World

```yaml
apiVersion: geminiset.io/v1alpha1
kind: GeminiSet
metadata:
  name: nodejs-hello-world
  namespace: default
spec:
  prompt: "Deploy 3 replicas of a hello world Node.js web application."
  constraints:
    - "distribute replicas across different availability zones in the cluster"
    - "do not place multiple replicas on the same physical worker node"
    - "limit memory usage to 128Mi per pod"
```

```bash
kubectl apply -f examples/01-nodejs-hello-world.yaml

kubectl get geminisets -n default
NAMESPACE   NAME                 REPLICAS   READY   AI-STATUS   AGE
default     nodejs-hello-world   3          3       Completed   30s

kubectl get pods -n default
NAME                       READY   STATUS    RESTARTS   AGE
nodejs-hello-world-12916   1/1     Running   0          59s
nodejs-hello-world-14cba   1/1     Running   0          59s
nodejs-hello-world-c343a   1/1     Running   0          59s

# To see the scheduling rationale score card:
kubectl geminiset why nodejs-hello-world-12916

# Delete the GeminiSet:
kubectl delete -f examples/01-nodejs-hello-world.yaml

# Watch the pods are terminating.
kubectl get pods -n default
NAME                       READY   STATUS        RESTARTS   AGE
nodejs-hello-world-39948   1/1     Terminating   0          8m9s
nodejs-hello-world-90c8b   1/1     Terminating   0          8m9s
nodejs-hello-world-992e9   1/1     Terminating   0
```

### Multilingual Manifests

```yaml
---
apiVersion: geminiset.io/v1alpha1
kind: GeminiSet
metadata:
  name: spanish-web-service
  namespace: default
spec:
  prompt: "Desplegar 3 réplicas de servidor web caddy para alta disponibilidad."
  constraints:
    - "distribuir equitativamente entre diferentes zonas de disponibilidad del clúster"
    - "evitar nodos con alta densidad de pods o sobrecargados de CPU"
    - "limitar el uso de memoria a 128Mi por pod"

---
apiVersion: geminiset.io/v1alpha1
kind: GeminiSet
metadata:
  name: japanese-hello-world
  namespace: default
spec:
  prompt: "3つのNode.js Hello Worldサーバーを稼働させ、高可用性を維持する。"
  constraints:
    - "クラスタ内の異なるノードに分散配置する"
    - "メモリ使用量を128Mi以内に抑える"
```

```bash
kubectl apply -f examples/02-polyglot-multilingual.yaml
```

### Zone Anti-Affinity

```yaml
apiVersion: geminiset.io/v1alpha1
kind: GeminiSet
metadata:
  name: zone-resilient-frontend
  namespace: default
spec:
  prompt: "Deploy 4 replicas of a fast static web server using nginx alpine."
  constraints:
    - "distribute pods evenly across all available zones in the cluster (zone anti-affinity)"
    - "do not place multiple frontend replicas on the same physical worker node"
    - "keep memory usage strictly under 128Mi per pod"
    - "avoid co-locating on nodes running database workloads"
```

```bash
kubectl apply -f examples/03-zone-resilient-frontend.yaml
```

### Old School Spec

You can also specify explicit container specifications and replica counts, while letting Gemini evaluate natural language placement constraints for pod scheduling:

```yaml
apiVersion: geminiset.io/v1alpha1
kind: GeminiSet
metadata:
  name: old-school-template-service
  namespace: default
spec:
  replicas: 3
  constraints:
    - "distribute pods evenly across all availability zones in the cluster"
    - "do not place multiple replicas on the same physical worker node"
    - "select nodes with lowest pod density and ample memory headroom"
  template:
    metadata:
      labels:
        app: custom-api
    spec:
      containers:
        - name: web
          image: nginx:alpine
          ports:
            - containerPort: 80
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi
```

```bash
kubectl apply -f examples/04-old-school-template.yaml
```

### Model Context Protocol (MCP) Tool Integration

GeminiSets can connect to external [MCP server](https://modelcontextprotocol.io) endpoints. The operator dynamically discovers tools and lets Gemini call external APIs during workload synthesis and scheduling:

```yaml
apiVersion: geminiset.io/v1alpha1
kind: GeminiSet
metadata:
  name: mcp-context-aware-service
  namespace: default
spec:
  prompt: "Deploy 3 replicas of the payment service with high-availability zone spread."
  constraints:
    - "distribute pods evenly across all availability zones in the cluster"
    - "select nodes with lowest pod density and ample memory headroom"
  
  # Connect external MCP servers for dynamic tool calling
  mcpServers:
    - name: prometheus-metrics
      endpoint: "http://prometheus-mcp.monitoring.svc.cluster.local:8080"
      description: "Live cluster metrics, node temperatures, and network latency"
```

```bash
kubectl apply -f examples/05-mcp-context-aware.yaml
```

## Scheduling Decision Rationale Card

When a pod is scheduled by Gemini Flash, detailed scoring matrices and chain-of-thought rationales are attached to the Pod annotations (`geminiset.io/decision-rationale`, `geminiset.io/node-scores`):

```
Gemini Scheduling Decision
────────────────────────────────────────────────────────────
Pod:           web-service-4d050
Assigned Node: node-eu-west3-b
Scores:
  • ResourceFit:         [███████████████] 100/100
  • TopologySpread:      [██████████████░]  95/100
  • LatencyOptimization: [██████████████░]  95/100
Rationale:
  node-eu-west3-b is the optimal match among cluster worker nodes because it
  satisfies the zone anti-affinity constraint and provides 14GiB available memory.
Alternatives Evaluated:
  • Node node-eu-west3-a (Score: 82/100):
    Satisfies anti-affinity, but has higher pod density.
  • Node node-eu-west3-c (Score: 80/100):
    Node has less available allocatable memory.
```

## Deployment

To deploy on a live Kubernetes cluster (GKE, EKS, Minikube, Kind):

```bash
# 1. Install CRDs
kubectl apply -f deploy/crds/geminiset.io_geminisets.yaml

# 2. Apply RBAC & Controller/Scheduler Operator
kubectl apply -f deploy/rbac.yaml
```

[`ko`](https://ko.build/) builds distroless container images:

```bash
# Build and deploy directly to the active Kubernetes cluster
KO_DOCKER_REPO=gcr.io/$GOOGLE_CLOUD_PROJECT/geminiset ko apply -f deploy/operator.yaml
```

### Configuring the Gemini API Key

Create a Kubernetes secret containing your Google Gemini API key in the `geminiset-system` namespace:

```bash
kubectl create secret generic gemini-credentials -n geminiset-system \
  --from-literal=api-key="YOUR_GEMINI_API_KEY"
```

The operator reads the key from this secret and defaults to using `gemini-3.7-flash`. You can customize the model by modifying the `GEMINI_MODEL` environment variable in `deploy/operator.yaml`.

## Troubleshooting

Helpful commands for inspecting and diagnosing GeminiSet workloads:

```bash
# List all GeminiSets and their status across namespaces
kubectl get geminisets -A

# View detailed AI synthesis metadata, condition status, and errors
kubectl describe geminiset <geminiset-name>

# Stream operator logs in the cluster
kubectl logs -n geminiset-system -l app=gemini-operator -f

# Run operator locally with verbose logging against your active kubeconfig
gemini-operator

# View Kubernetes events emitted by the Gemini scheduler
kubectl get events --field-selector source=gemini-scheduler -A

# View annotations (score matrix, decision rationale) on a scheduled pod
kubectl get pod <pod-name> -o yaml | grep -A 10 "geminiset.io/"

# Use CLI to inspect scheduling explanation
kubectl geminiset why <pod-name>

# Check pending pods waiting for scheduling
kubectl get pods --field-selector status.phase=Pending -A

# Describe pod to see scheduling failure events
kubectl describe pod <pod-name>
```

## Cost

Gemini models are not polled continuously; calls are event-driven and cached:

- **Workload Synthesis**: Gemini is called only when a GeminiSet is created or its specification changes. The controller caches synthesized templates using a SHA-256 hash to skip model calls during routine reconciliations.
- **Pod Scheduling**: Gemini is called only when unscheduled pods exist. Once all pods in the cluster are scheduled, the scheduler performs zero API calls.

Workload synthesis is intentionally decoupled from scheduling to preserve Kubernetes' separation of concerns. Synthesizing the specification once allows the controller to cache container templates, so dynamic lifecycle events like replica scaling, pod restarts, or node evictions only invoke the scheduler without re-synthesizing the workload. While this adds to the overall number of Gemini calls, it provides a much more resilient approach.

Consequently, model invocation costs scale linearly with the number of replicas ($1$ synthesis call + $N$ scheduling calls for $N$ replicas). Once all replicas are scheduled and running in steady state, ongoing model costs drop to zero.
