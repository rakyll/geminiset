package testutil

import (
	"context"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
)

// InMemoryGeminiSetClient provides an in-memory repository for GeminiSets.
type InMemoryGeminiSetClient struct {
	mu   sync.RWMutex
	sets map[string]*geminiv1alpha1.GeminiSet
}

func NewInMemoryGeminiSetClient() *InMemoryGeminiSetClient {
	return &InMemoryGeminiSetClient{
		sets: make(map[string]*geminiv1alpha1.GeminiSet),
	}
}

func (c *InMemoryGeminiSetClient) Create(ctx context.Context, set *geminiv1alpha1.GeminiSet) (*geminiv1alpha1.GeminiSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s/%s", set.Namespace, set.Name)
	if set.Namespace == "" {
		set.Namespace = "default"
		key = fmt.Sprintf("default/%s", set.Name)
	}
	if set.CreationTimestamp.IsZero() {
		set.CreationTimestamp = metav1.Now()
	}
	c.sets[key] = set.DeepCopy()
	return set.DeepCopy(), nil
}

func (c *InMemoryGeminiSetClient) Get(ctx context.Context, namespace, name string) (*geminiv1alpha1.GeminiSet, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if namespace == "" {
		namespace = "default"
	}
	key := fmt.Sprintf("%s/%s", namespace, name)
	set, ok := c.sets[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "geminiset.io", Resource: "geminisets"}, name)
	}
	return set.DeepCopy(), nil
}

func (c *InMemoryGeminiSetClient) List(ctx context.Context, namespace string) (*geminiv1alpha1.GeminiSetList, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var items []geminiv1alpha1.GeminiSet
	for _, set := range c.sets {
		if namespace == "" || set.Namespace == namespace {
			items = append(items, *set.DeepCopy())
		}
	}
	return &geminiv1alpha1.GeminiSetList{Items: items}, nil
}

func (c *InMemoryGeminiSetClient) Update(ctx context.Context, set *geminiv1alpha1.GeminiSet) (*geminiv1alpha1.GeminiSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s/%s", set.Namespace, set.Name)
	c.sets[key] = set.DeepCopy()
	return set.DeepCopy(), nil
}

func (c *InMemoryGeminiSetClient) UpdateStatus(ctx context.Context, set *geminiv1alpha1.GeminiSet) (*geminiv1alpha1.GeminiSet, error) {
	return c.Update(ctx, set)
}

func (c *InMemoryGeminiSetClient) Delete(ctx context.Context, namespace, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, name)
	delete(c.sets, key)
	return nil
}

// TestCluster contains initialized fake clients and nodes for testing and simulation.
type TestCluster struct {
	KubeClient *fake.Clientset
	GeminiSets *InMemoryGeminiSetClient
}

func NewTestCluster() *TestCluster {
	kubeClient := fake.NewSimpleClientset()
	geminiSets := NewInMemoryGeminiSetClient()

	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-eu-west3-a",
				Labels: map[string]string{
					"topology.kubernetes.io/zone":     "europe-west3-a",
					"topology.kubernetes.io/region":   "europe-west3",
					"node.kubernetes.io/instance-type": "n2-standard-4",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3800m"),
					corev1.ResourceMemory: resource.MustParse("14Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-eu-west3-b",
				Labels: map[string]string{
					"topology.kubernetes.io/zone":     "europe-west3-b",
					"topology.kubernetes.io/region":   "europe-west3",
					"node.kubernetes.io/instance-type": "n2-standard-4",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3800m"),
					corev1.ResourceMemory: resource.MustParse("14Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-us-west1-a",
				Labels: map[string]string{
					"topology.kubernetes.io/zone":     "us-west1-a",
					"topology.kubernetes.io/region":   "us-west1",
					"node.kubernetes.io/instance-type": "e2-standard-4",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3800m"),
					corev1.ResourceMemory: resource.MustParse("14Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-us-east4-a",
				Labels: map[string]string{
					"topology.kubernetes.io/zone":     "us-east4-a",
					"topology.kubernetes.io/region":   "us-east4",
					"node.kubernetes.io/instance-type": "m1-megamem-96",
				},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("16"),
					corev1.ResourceMemory: resource.MustParse("64Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("15000m"),
					corev1.ResourceMemory: resource.MustParse("60Gi"),
				},
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
	}

	for _, n := range nodes {
		_, _ = kubeClient.CoreV1().Nodes().Create(context.Background(), n, metav1.CreateOptions{})
	}

	return &TestCluster{
		KubeClient: kubeClient,
		GeminiSets: geminiSets,
	}
}
