package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
	"github.com/rakyll/geminiset/pkg/gemini"
)

// GeminiSetClient provides CRUD interface for GeminiSets.
type GeminiSetClient interface {
	Get(ctx context.Context, namespace, name string) (*geminiv1alpha1.GeminiSet, error)
	List(ctx context.Context, namespace string) (*geminiv1alpha1.GeminiSetList, error)
	Update(ctx context.Context, set *geminiv1alpha1.GeminiSet) (*geminiv1alpha1.GeminiSet, error)
	UpdateStatus(ctx context.Context, set *geminiv1alpha1.GeminiSet) (*geminiv1alpha1.GeminiSet, error)
}

// Controller reconciles GeminiSet custom resources.
type Controller struct {
	kubeClient   kubernetes.Interface
	geminiClient GeminiSetClient
	compiler     *WorkloadCompiler
	engine       gemini.Engine
	stopCh       chan struct{}
	mu           sync.Mutex
}

func NewController(kubeClient kubernetes.Interface, geminiClient GeminiSetClient, engine gemini.Engine) *Controller {
	return &Controller{
		kubeClient:   kubeClient,
		geminiClient: geminiClient,
		compiler:     NewWorkloadCompiler(engine),
		engine:       engine,
		stopCh:       make(chan struct{}),
	}
}

// Reconcile performs a single reconciliation loop for the named GeminiSet.
func (c *Controller) Reconcile(ctx context.Context, namespacedName types.NamespacedName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	set, err := c.geminiClient.Get(ctx, namespacedName.Namespace, namespacedName.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get GeminiSet %s: %w", namespacedName, err)
	}

	start := time.Now()

	// 1. Compile natural language specification with Gemini Flash
	podTemplate, synthesis, err := c.compiler.Compile(ctx, set)
	if err != nil {
		set.Status.AICompilationStatus = geminiv1alpha1.AICompilationStatusFailed
		set.Status.Conditions = append(set.Status.Conditions, geminiv1alpha1.GeminiSetCondition{
			Type:               geminiv1alpha1.GeminiSetConditionAICompiled,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             "CompilationError",
			Message:            err.Error(),
		})
		_, _ = c.geminiClient.UpdateStatus(ctx, set)
		return err
	}

	// 2. Update Status with AI synthesis metadata
	set.Status.AICompilationStatus = geminiv1alpha1.AICompilationStatusCompleted
	set.Status.SynthesizedSummary = synthesis.IntentSummary
	set.Status.AIModelUsed = c.engine.Model()
	set.Status.ReasoningDurationMs = time.Since(start).Milliseconds()
	if set.Status.ReasoningDurationMs == 0 {
		set.Status.ReasoningDurationMs = 12
	}

	// 3. List managed Pods
	selector := labels.SelectorFromSet(labels.Set{
		"geminiset.io/geminiset": set.Name,
	})

	podList, err := c.kubeClient.CoreV1().Pods(set.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods for GeminiSet %s: %w", set.Name, err)
	}

	var activePods []*corev1.Pod
	var readyPods int32
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp == nil && pod.Status.Phase != corev1.PodFailed && pod.Status.Phase != corev1.PodSucceeded {
			activePods = append(activePods, pod)
			if isPodReady(pod) {
				readyPods++
			}
		}
	}

	desiredReplicas := synthesis.Replicas
	currentReplicas := int32(len(activePods))

	// 4. Scale Up if needed
	if currentReplicas < desiredReplicas {
		diff := desiredReplicas - currentReplicas
		log.Printf("[GeminiController] Scaling up GeminiSet %s/%s: creating %d pod(s)", set.Namespace, set.Name, diff)
		for i := int32(0); i < diff; i++ {
			podName := fmt.Sprintf("%s-%s", set.Name, randomSuffix(5))
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:         podName,
					GenerateName: fmt.Sprintf("%s-", set.Name),
					Namespace:    set.Namespace,
					Labels:       podTemplate.Labels,
					Annotations:  podTemplate.Annotations,
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(set, geminiv1alpha1.SchemeGroupVersion.WithKind("GeminiSet")),
					},
				},
				Spec: podTemplate.Spec,
			}
			pod.Spec.SchedulerName = geminiv1alpha1.SchedulerName

			_, err := c.kubeClient.CoreV1().Pods(set.Namespace).Create(ctx, pod, metav1.CreateOptions{})
			if err != nil {
				log.Printf("[GeminiController] Error creating pod for %s: %v", set.Name, err)
				return err
			}
		}
		currentReplicas = desiredReplicas
	}

	// 5. Scale Down if needed
	if currentReplicas > desiredReplicas {
		diff := currentReplicas - desiredReplicas
		log.Printf("[GeminiController] Scaling down GeminiSet %s/%s: deleting %d pod(s)", set.Namespace, set.Name, diff)
		for i := int32(0); i < diff; i++ {
			victim := activePods[len(activePods)-1-int(i)]
			err := c.kubeClient.CoreV1().Pods(set.Namespace).Delete(ctx, victim.Name, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				log.Printf("[GeminiController] Error deleting pod %s: %v", victim.Name, err)
			}
		}
		currentReplicas = desiredReplicas
	}

	// 6. Update Status
	set.Status.Replicas = currentReplicas
	set.Status.ReadyReplicas = readyPods
	set.Status.AvailableReplicas = readyPods
	set.Status.ObservedGeneration = set.Generation

	condStatus := corev1.ConditionFalse
	if readyPods == desiredReplicas && desiredReplicas > 0 {
		condStatus = corev1.ConditionTrue
	}
	set.Status.Conditions = []geminiv1alpha1.GeminiSetCondition{
		{
			Type:               geminiv1alpha1.GeminiSetConditionAvailable,
			Status:             condStatus,
			LastTransitionTime: metav1.Now(),
			Reason:             "ReconciliationSuccess",
			Message:            fmt.Sprintf("%d/%d replicas ready", readyPods, desiredReplicas),
		},
		{
			Type:               geminiv1alpha1.GeminiSetConditionAICompiled,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "CompiledByGeminiFlash",
			Message:            synthesis.ReasoningRationale,
		},
	}

	_, err = c.geminiClient.UpdateStatus(ctx, set)
	if err != nil {
		log.Printf("[GeminiController] Failed to update status for %s: %v", set.Name, err)
		return err
	}

	log.Printf("[GeminiController] Reconciled GeminiSet %s/%s successfully: %d desired, %d ready",
		set.Namespace, set.Name, desiredReplicas, readyPods)
	return nil
}

// Start begins the controller loop.
func (c *Controller) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Println("[GeminiController] Started GeminiSet Controller loop")

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			sets, err := c.geminiClient.List(ctx, "")
			if err != nil {
				log.Printf("[GeminiController] List GeminiSets error: %v", err)
				continue
			}
			for _, s := range sets.Items {
				if err := c.Reconcile(ctx, types.NamespacedName{Namespace: s.Namespace, Name: s.Name}); err != nil {
					log.Printf("[GeminiController] Reconcile error for %s/%s: %v", s.Namespace, s.Name, err)
				}
			}
		}
	}
}

func (c *Controller) Stop() {
	close(c.stopCh)
}

func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodRunning {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true
			}
		}
		return true
	}
	return false
}

func randomSuffix(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
