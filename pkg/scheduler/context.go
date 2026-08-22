package scheduler

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/rakyll/geminiset/pkg/gemini"
)

// ClusterContextCollector gathers live telemetry and topology from the Kubernetes cluster.
type ClusterContextCollector struct {
	kubeClient kubernetes.Interface
}

func NewClusterContextCollector(kubeClient kubernetes.Interface) *ClusterContextCollector {
	return &ClusterContextCollector{kubeClient: kubeClient}
}

// CollectTelemetry scans nodes and pods to create an AI-ready snapshot for Gemini Flash.
func (c *ClusterContextCollector) CollectTelemetry(ctx context.Context) ([]gemini.NodeTelemetry, error) {
	nodeList, err := c.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	podList, err := c.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Map existing pods to node names
	podsPerNode := make(map[string][]string)
	for _, p := range podList.Items {
		if p.Spec.NodeName != "" && p.Status.Phase != corev1.PodSucceeded && p.Status.Phase != corev1.PodFailed {
			podsPerNode[p.Spec.NodeName] = append(podsPerNode[p.Spec.NodeName], p.Name)
		}
	}

	var results []gemini.NodeTelemetry
	for _, node := range nodeList.Items {
		zone := node.Labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = node.Labels["failure-domain.beta.kubernetes.io/zone"]
		}
		if zone == "" {
			zone = "zone-a"
		}

		region := node.Labels["topology.kubernetes.io/region"]
		if region == "" {
			region = node.Labels["failure-domain.beta.kubernetes.io/region"]
		}
		if region == "" {
			region = "region-1"
		}

		instanceType := node.Labels["node.kubernetes.io/instance-type"]
		if instanceType == "" {
			instanceType = "standard-4cpu-16gb"
		}

		cpuAlloc := node.Status.Allocatable.Cpu()
		memAlloc := node.Status.Allocatable.Memory()

		var cpuMilliFree int64 = 4000
		var memMBFree int64 = 8192
		if cpuAlloc != nil {
			cpuMilliFree = cpuAlloc.MilliValue()
		}
		if memAlloc != nil {
			memMBFree = memAlloc.Value() / (1024 * 1024)
		}

		existing := podsPerNode[node.Name]

		var taints []string
		for _, t := range node.Spec.Taints {
			taints = append(taints, fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect))
		}

		telemetry := gemini.NodeTelemetry{
			Name:              node.Name,
			Zone:              zone,
			Region:            region,
			InstanceType:      instanceType,
			CPUCapacity:       node.Status.Capacity.Cpu().String(),
			CPUAllocatable:    node.Status.Allocatable.Cpu().String(),
			MemoryCapacity:    node.Status.Capacity.Memory().String(),
			MemoryAllocatable: node.Status.Allocatable.Memory().String(),
			CPUMilliFree:      cpuMilliFree,
			MemoryMBFree:      memMBFree,
			ExistingPods:      existing,
			PodCount:          len(existing),
			Labels:            node.Labels,
			Taints:            taints,
		}

		results = append(results, telemetry)
	}

	return results, nil
}
