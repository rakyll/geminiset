package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
	"github.com/rakyll/geminiset/pkg/testutil"
)

func TestController_Reconciliation(t *testing.T) {
	cluster := testutil.NewTestCluster()
	engine := testutil.NewMockEngine()
	ctrl := NewController(cluster.KubeClient, cluster.GeminiSets, engine)

	ctx := context.Background()

	// 1. Create Turkish GeminiSet with prompt-only spec
	set := &geminiv1alpha1.GeminiSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-turkce-set",
			Namespace: "default",
		},
		Spec: geminiv1alpha1.GeminiSetSpec{
			Prompt: "3 adet yüksek erişilebilirlikli redis önbellek sunucusu çalıştır ve farklı düğümlere dağıt.",
		},
	}

	_, err := cluster.GeminiSets.Create(ctx, set)
	if err != nil {
		t.Fatalf("failed to create geminiset: %v", err)
	}

	// 2. Run Reconcile
	err = ctrl.Reconcile(ctx, types.NamespacedName{Namespace: "default", Name: "test-turkce-set"})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// 3. Verify Pods created
	podList, err := cluster.KubeClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}

	if len(podList.Items) != 3 {
		t.Fatalf("expected 3 pods to be created, got %d", len(podList.Items))
	}

	// Verify all pods have schedulerName: gemini-scheduler
	for _, p := range podList.Items {
		if p.Spec.SchedulerName != geminiv1alpha1.SchedulerName {
			t.Errorf("pod %s has wrong schedulerName: %s (expected %s)",
				p.Name, p.Spec.SchedulerName, geminiv1alpha1.SchedulerName)
		}
	}

	// 4. Verify GeminiSet Status
	updatedSet, err := cluster.GeminiSets.Get(ctx, "default", "test-turkce-set")
	if err != nil {
		t.Fatalf("failed to get updated geminiset: %v", err)
	}

	if updatedSet.Status.Replicas != 3 {
		t.Errorf("expected status replicas 3, got %d", updatedSet.Status.Replicas)
	}
	if updatedSet.Status.AICompilationStatus != geminiv1alpha1.AICompilationStatusCompleted {
		t.Errorf("expected AICompilationStatus %s, got %s", geminiv1alpha1.AICompilationStatusCompleted, updatedSet.Status.AICompilationStatus)
	}
}
