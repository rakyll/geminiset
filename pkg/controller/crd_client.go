package controller

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	geminiv1alpha1 "github.com/rakyll/geminiset/pkg/api/v1alpha1"
)

var (
	geminiSetGVR = schema.GroupVersionResource{
		Group:    geminiv1alpha1.GroupName,
		Version:  geminiv1alpha1.Version,
		Resource: "geminisets",
	}
)

// DynamicGeminiSetClient wraps Kubernetes dynamic.Interface to provide typed GeminiSet access.
type DynamicGeminiSetClient struct {
	dyn dynamic.Interface
}

func NewDynamicGeminiSetClient(dyn dynamic.Interface) *DynamicGeminiSetClient {
	return &DynamicGeminiSetClient{dyn: dyn}
}

func (c *DynamicGeminiSetClient) Get(ctx context.Context, namespace, name string) (*geminiv1alpha1.GeminiSet, error) {
	if namespace == "" {
		namespace = "default"
	}
	u, err := c.dyn.Resource(geminiSetGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	var set geminiv1alpha1.GeminiSet
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &set)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to GeminiSet: %w", err)
	}
	return &set, nil
}

func (c *DynamicGeminiSetClient) List(ctx context.Context, namespace string) (*geminiv1alpha1.GeminiSetList, error) {
	var uList *unstructured.UnstructuredList
	var err error
	if namespace == "" {
		uList, err = c.dyn.Resource(geminiSetGVR).List(ctx, metav1.ListOptions{})
	} else {
		uList, err = c.dyn.Resource(geminiSetGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	var list geminiv1alpha1.GeminiSetList
	for _, item := range uList.Items {
		var set geminiv1alpha1.GeminiSet
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &set); err == nil {
			list.Items = append(list.Items, set)
		}
	}
	return &list, nil
}

func (c *DynamicGeminiSetClient) Update(ctx context.Context, set *geminiv1alpha1.GeminiSet) (*geminiv1alpha1.GeminiSet, error) {
	ns := set.Namespace
	if ns == "" {
		ns = "default"
	}

	raw, err := json.Marshal(set)
	if err != nil {
		return nil, err
	}
	var unstr map[string]interface{}
	if err := json.Unmarshal(raw, &unstr); err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{Object: unstr}
	updatedU, err := c.dyn.Resource(geminiSetGVR).Namespace(ns).Update(ctx, u, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	var updatedSet geminiv1alpha1.GeminiSet
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(updatedU.Object, &updatedSet)
	return &updatedSet, nil
}

func (c *DynamicGeminiSetClient) UpdateStatus(ctx context.Context, set *geminiv1alpha1.GeminiSet) (*geminiv1alpha1.GeminiSet, error) {
	ns := set.Namespace
	if ns == "" {
		ns = "default"
	}

	raw, err := json.Marshal(set)
	if err != nil {
		return nil, err
	}
	var unstr map[string]interface{}
	if err := json.Unmarshal(raw, &unstr); err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{Object: unstr}
	updatedU, err := c.dyn.Resource(geminiSetGVR).Namespace(ns).UpdateStatus(ctx, u, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	var updatedSet geminiv1alpha1.GeminiSet
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(updatedU.Object, &updatedSet)
	return &updatedSet, nil
}
