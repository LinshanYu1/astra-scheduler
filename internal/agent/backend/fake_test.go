package backend

import (
	"context"
	"testing"

	astrav1alpha1 "github.com/linshanyu/astra-scheduler/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFakeBackendSnapshotReadsNodeResourcesFromConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := astrav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add astra scheme: %v", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "astra-system",
			Name:      "fake-node-resources",
		},
		Data: map[string]string{
			"node-a.yaml": `
allocatable:
  gpuCount: 4
  gpuMemoryGiB: 320
  kvCacheGiB: 128
  prefillTokensPerSecond: 6000
  decodeTokensPerSecond: 9000
runtime:
  pressure:
    sloRisk: low
    queueDepth: 2
`,
		},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
	backend := NewFakeBackend(Options{
		Client:             reader,
		NodeName:           "node-a",
		ConfigMapNamespace: "astra-system",
		ConfigMapName:      "fake-node-resources",
	})
	allocationResources := &astrav1alpha1.ResourceSummary{
		GPUCount:              1,
		KVCacheGiB:            16,
		DecodeTokensPerSecond: 1000,
	}

	result, err := backend.Snapshot(context.Background(), SnapshotRequest{
		Node: &astrav1alpha1.AINodeResourceProfile{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "astra-system",
				Name:      "node-a",
			},
			Spec: astrav1alpha1.AINodeResourceProfileSpec{
				NodeName: "node-a",
			},
		},
		Allocations: []astrav1alpha1.AIResourceAllocation{{
			ObjectMeta: metav1.ObjectMeta{Name: "alloc-a", Namespace: "default"},
			Spec: astrav1alpha1.AIResourceAllocationSpec{
				Resources: allocationResources,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}

	if result.Status.Allocatable.GPUCount != 4 {
		t.Fatalf("expected ConfigMap GPU capacity 4, got %d", result.Status.Allocatable.GPUCount)
	}
	if result.Status.Available.DecodeTokensPerSecond != 8000 {
		t.Fatalf("expected available decode tokens 8000, got %d", result.Status.Available.DecodeTokensPerSecond)
	}
	if result.Status.Runtime == nil || result.Status.Runtime.Pressure == nil || result.Status.Runtime.Pressure.QueueDepth != 2 {
		t.Fatalf("expected runtime pressure from ConfigMap, got %#v", result.Status.Runtime)
	}
}
