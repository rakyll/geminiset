package scheduler

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
	"github.com/rakyll/geminiset/pkg/testutil"
)

func TestScheduler_ScheduleNext(t *testing.T) {
	cluster := testutil.NewTestCluster()
	engine := testutil.NewMockEngine()
	sched := NewScheduler(cluster.KubeClient, engine)
	ctx := context.Background()

	// 1. Create unscheduled pod requiring gemini-scheduler
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-west",
			Namespace: "default",
			Annotations: map[string]string{
				geminiv1alpha1.AnnotationPlacementHint: "region=europe-west3",
			},
		},
		Spec: corev1.PodSpec{
			SchedulerName: geminiv1alpha1.SchedulerName,
			Containers: []corev1.Container{
				{
					Name:  "web",
					Image: "nginx:alpine",
				},
			},
		},
	}

	_, err := cluster.KubeClient.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create test pod: %v", err)
	}

	// 2. Run ScheduleNext
	err = sched.ScheduleNext(ctx)
	if err != nil {
		t.Fatalf("ScheduleNext failed: %v", err)
	}

	// 3. Verify Pod was bound to node in europe-west3
	scheduledPod, err := cluster.KubeClient.CoreV1().Pods("default").Get(ctx, "test-pod-west", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get pod: %v", err)
	}

	if scheduledPod.Spec.NodeName != "node-eu-west3-a" && scheduledPod.Spec.NodeName != "node-eu-west3-b" {
		t.Errorf("expected pod to be scheduled to europe-west3 node, got %s", scheduledPod.Spec.NodeName)
	}

	if scheduledPod.Annotations[geminiv1alpha1.AnnotationDecisionRationale] == "" {
		t.Errorf("expected decision rationale annotation to be present")
	}

	if scheduledPod.Annotations[geminiv1alpha1.AnnotationScheduledBy] == "" {
		t.Errorf("expected scheduled-by annotation to be present")
	}
}
