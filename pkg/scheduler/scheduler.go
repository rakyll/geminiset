package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
	"github.com/rakyll/geminiset/pkg/gemini"
)

// Scheduler watches unscheduled Pods assigned to "gemini-scheduler"
// and schedules them using the Gemini Flash AI engine.
type Scheduler struct {
	kubeClient kubernetes.Interface
	engine     gemini.Engine
	collector  *ClusterContextCollector
	stopCh     chan struct{}
	mu         sync.Mutex
}

func NewScheduler(kubeClient kubernetes.Interface, engine gemini.Engine) *Scheduler {
	return &Scheduler{
		kubeClient: kubeClient,
		engine:     engine,
		collector:  NewClusterContextCollector(kubeClient),
		stopCh:     make(chan struct{}),
	}
}

// ScheduleNext finds unscheduled pods and runs the AI scheduling pipeline.
func (s *Scheduler) ScheduleNext(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	podList, err := s.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	var pendingPods []*corev1.Pod
	for i := range podList.Items {
		p := &podList.Items[i]
		if p.Spec.SchedulerName == geminiv1alpha1.SchedulerName && p.Spec.NodeName == "" && p.DeletionTimestamp == nil {
			pendingPods = append(pendingPods, p)
		}
	}

	if len(pendingPods) == 0 {
		return nil
	}

	nodes, err := s.collector.CollectTelemetry(ctx)
	if err != nil {
		return fmt.Errorf("failed to collect node telemetry: %w", err)
	}

	if len(nodes) == 0 {
		return fmt.Errorf("no candidate nodes available in cluster")
	}

	for _, pod := range pendingPods {
		if err := s.scheduleSinglePod(ctx, pod, nodes); err != nil {
			log.Printf("[GeminiScheduler] Failed to schedule pod %s/%s: %v", pod.Namespace, pod.Name, err)
		}
	}

	return nil
}

func (s *Scheduler) scheduleSinglePod(ctx context.Context, pod *corev1.Pod, candidateNodes []gemini.NodeTelemetry) error {
	ownerGeminiSet := pod.Labels["geminiset.io/geminiset"]
	if ownerGeminiSet == "" {
		ownerGeminiSet = pod.Labels["geminiset.io/workload"]
	}

	var reqCPU, reqMem string
	for _, c := range pod.Spec.Containers {
		if c.Resources.Requests != nil {
			if cpu := c.Resources.Requests.Cpu(); cpu != nil {
				reqCPU = cpu.String()
			}
			if mem := c.Resources.Requests.Memory(); mem != nil {
				reqMem = mem.String()
			}
		}
	}

	var mcpServers []geminiv1alpha1.MCPServerSpec
	if mcpJSON, ok := pod.Annotations[geminiv1alpha1.AnnotationMCPServers]; ok && mcpJSON != "" {
		_ = json.Unmarshal([]byte(mcpJSON), &mcpServers)
	}

	req := gemini.SchedulingRequest{
		PodName:         pod.Name,
		PodNamespace:    pod.Namespace,
		OwnerGeminiSet:  ownerGeminiSet,
		RequestedCPU:    reqCPU,
		RequestedMemory: reqMem,
		PlacementHint:   pod.Annotations[geminiv1alpha1.AnnotationPlacementHint],
		PodAnnotations:  pod.Annotations,
		CandidateNodes:  candidateNodes,
		MCPServers:      mcpServers,
	}

	log.Printf("[GeminiScheduler] Invoking Gemini Flash 3.7 to schedule pod %s/%s (Hint: %s)...",
		pod.Namespace, pod.Name, req.PlacementHint)

	decision, err := s.engine.SchedulePod(ctx, req)
	if err != nil {
		s.emitEvent(ctx, pod, corev1.EventTypeWarning, "FailedScheduling", fmt.Sprintf("Gemini scheduling failed: %v", err))
		return err
	}

	log.Printf("[GeminiScheduler] Decision for %s: Target Node '%s' (Confidence: %.2f, Duration: %dms)",
		pod.Name, decision.SelectedNode, decision.Confidence, decision.DurationMs)
	log.Printf("[GeminiScheduler] AI Rationale: %s", decision.DecisionRationale)

	// 1. Perform Binding to Node
	binding := &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			UID:       pod.UID,
		},
		Target: corev1.ObjectReference{
			Kind: "Node",
			Name: decision.SelectedNode,
		},
	}

	if err := s.kubeClient.CoreV1().Pods(pod.Namespace).Bind(ctx, binding, metav1.CreateOptions{}); err != nil {
		log.Printf("[GeminiScheduler] Binding call: %v", err)
	}

	// 2. Ensure Pod object reflects NodeName and rich AI annotations
	podUpdated, getErr := s.kubeClient.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	if getErr == nil {
		podUpdated.Spec.NodeName = decision.SelectedNode
		if podUpdated.Annotations == nil {
			podUpdated.Annotations = make(map[string]string)
		}
		podUpdated.Annotations[geminiv1alpha1.AnnotationScheduledBy] = decision.ModelUsed
		podUpdated.Annotations[geminiv1alpha1.AnnotationScheduledAt] = time.Now().Format(time.RFC3339)
		podUpdated.Annotations[geminiv1alpha1.AnnotationDecisionRationale] = decision.DecisionRationale

		if scoresJSON, err := json.Marshal(decision.ScoreMatrix); err == nil {
			podUpdated.Annotations[geminiv1alpha1.AnnotationNodeScores] = string(scoresJSON)
		}
		if altsJSON, err := json.Marshal(decision.AlternativesEvaluated); err == nil {
			podUpdated.Annotations[geminiv1alpha1.AnnotationAlternativeNodes] = string(altsJSON)
		}

		_, err = s.kubeClient.CoreV1().Pods(pod.Namespace).Update(ctx, podUpdated, metav1.UpdateOptions{})
		if err != nil {
			log.Printf("[GeminiScheduler] Note: update pod: %v", err)
		}
	}

	// 3. Emit Kubernetes Event
	eventMsg := fmt.Sprintf("Successfully assigned %s/%s to node %s using Gemini Flash 3.7. Rationale: %s",
		pod.Namespace, pod.Name, decision.SelectedNode, decision.DecisionRationale)
	s.emitEvent(ctx, pod, corev1.EventTypeNormal, "ScheduledByGeminiFlash", eventMsg)

	// Update local candidate list so subsequent pods in the same batch see this assignment
	for i := range candidateNodes {
		if candidateNodes[i].Name == decision.SelectedNode {
			candidateNodes[i].ExistingPods = append(candidateNodes[i].ExistingPods, pod.Name)
			candidateNodes[i].PodCount++
		}
	}

	return nil
}

func (s *Scheduler) emitEvent(ctx context.Context, pod *corev1.Pod, eventType, reason, message string) {
	t := metav1.Time{Time: time.Now()}
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-sched-", pod.Name),
			Namespace:    pod.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Namespace: pod.Namespace,
			Name:      pod.Name,
			UID:       pod.UID,
		},
		Reason:         reason,
		Message:        message,
		Type:           eventType,
		FirstTimestamp: t,
		LastTimestamp:  t,
		Count:          1,
		Source: corev1.EventSource{
			Component: "gemini-scheduler",
		},
	}
	_, _ = s.kubeClient.CoreV1().Events(pod.Namespace).Create(ctx, event, metav1.CreateOptions{})
}

// Start runs the scheduler polling loop.
func (s *Scheduler) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Println("[GeminiScheduler] Started Gemini Flash AI Scheduler loop")

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			_ = s.ScheduleNext(ctx)
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
}
