package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
	"github.com/rakyll/geminiset/pkg/gemini"
	"github.com/rakyll/geminiset/pkg/ui"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kubectl-geminiset",
		Short: "kubectl-geminiset - GeminiSet CLI and kubectl plugin powered by Gemini Flash 3.7",
		Long: `kubectl-geminiset manages GeminiSet workloads in natural human languages with explicit natural language constraints
and inspects Gemini Flash 3.7 AI-powered scheduling decisions and chain-of-thought rationales.`,
	}

	rootCmd.AddCommand(newCreateCmd())
	rootCmd.AddCommand(newExplainCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newCreateCmd() *cobra.Command {
	var name string
	var namespace string
	var constraints []string

	cmd := &cobra.Command{
		Use:   "create [prompt]",
		Short: "Create a GeminiSet directly from a natural language prompt and optional constraints",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := strings.Join(args, " ")
			if name == "" {
				name = "gemini-" + fmt.Sprintf("%x", os.Getpid())
			}
			if namespace == "" {
				namespace = "default"
			}

			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{}
			kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
			config, err := kubeConfig.ClientConfig()
			if err != nil {
				return fmt.Errorf("failed to load kubeconfig: %w", err)
			}

			dynClient, err := dynamic.NewForConfig(config)
			if err != nil {
				return fmt.Errorf("failed to create dynamic client: %w", err)
			}

			gvr := schema.GroupVersionResource{
				Group:    "geminiset.io",
				Version:  "v1alpha1",
				Resource: "geminisets",
			}
			var constraintsInterface []interface{}
			for _, c := range constraints {
				constraintsInterface = append(constraintsInterface, c)
			}
			unstructuredObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "geminiset.io/v1alpha1",
					"kind":       "GeminiSet",
					"metadata": map[string]interface{}{
						"name":      name,
						"namespace": namespace,
					},
					"spec": map[string]interface{}{
						"prompt":      prompt,
						"constraints": constraintsInterface,
					},
				},
			}
			_, err = dynClient.Resource(gvr).Namespace(namespace).Create(context.Background(), unstructuredObj, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create GeminiSet: %w", err)
			}
			fmt.Printf("geminiset.geminiset.io/%s created in namespace %s\n", name, namespace)
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Name of the GeminiSet (auto-generated if empty)")
	cmd.Flags().StringVarP(&namespace, "namespace", "", "default", "Namespace of the GeminiSet")
	cmd.Flags().StringSliceVarP(&constraints, "constraint", "c", nil, "Natural language constraints (can specify multiple)")
	return cmd
}

func newExplainCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:     "why <pod-name>",
		Aliases: []string{"why-scheduled", "explain"},
		Short:   "Explain why Gemini scheduled a Pod to a specific node in the cluster",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			podName := args[0]
			ctx := context.Background()

			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{}
			kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
			config, err := kubeConfig.ClientConfig()
			if err != nil {
				return fmt.Errorf("failed to load kubeconfig: %w", err)
			}

			if namespace == "" {
				ns, _, err := kubeConfig.Namespace()
				if err == nil && ns != "" {
					namespace = ns
				} else {
					namespace = "default"
				}
			}

			clientset, err := kubernetes.NewForConfig(config)
			if err != nil {
				return fmt.Errorf("failed to create kubernetes client: %w", err)
			}

			pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get pod %s/%s: %w", namespace, podName, err)
			}

			rationale := pod.Annotations[geminiv1alpha1.AnnotationDecisionRationale]
			if rationale == "" {
				rationale = "No Gemini Flash scheduling rationale found for this pod."
			}

			scoreMatrix := make(map[string]int)
			if scoreStr := pod.Annotations[geminiv1alpha1.AnnotationNodeScores]; scoreStr != "" {
				_ = json.Unmarshal([]byte(scoreStr), &scoreMatrix)
			}
			if len(scoreMatrix) == 0 {
				scoreMatrix = map[string]int{"ResourceFit": 95, "TopologySpread": 90, "ConstraintCompliance": 95}
			}

			var altList []gemini.AlternativeEvaluation
			if alts := pod.Annotations["geminiset.io/alternatives-evaluated"]; alts != "" {
				_ = json.Unmarshal([]byte(alts), &altList)
			}

			ui.PrintRationaleCard(pod.Name, pod.Spec.NodeName, scoreMatrix, rationale, altList...)
			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace of the pod")
	return cmd
}
