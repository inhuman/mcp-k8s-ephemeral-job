//go:build envtest

// create/delete contract against a REAL kube-apiserver (the k8s API is never mocked).
// Run: envtest/CI provides the endpoint via KUBECONFIG (or in-cluster).
//
//	go test -tags envtest ./tests/integration -run TestContract
package integration

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/inhuman/mcp-k8s-ephemeral-job/internal/manifest"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func clientset(t *testing.T) kubernetes.Interface {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set; envtest/cluster endpoint required")
	}
	rc, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("rest config: %v", err)
	}
	cs, err := kubernetes.NewForConfig(rc)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	return cs
}

func TestContractCreateDelete(t *testing.T) {
	ctx := t.Context()
	cs := clientset(t)
	ns := "default"

	job, err := manifest.Build(manifest.Params{
		Namespace: ns, SidecarImage: "busybox:1.36", RunID: "contract-test", TTLSeconds: 60,
		Image: "busybox:1.36", Command: []string{"true"}, CPURequest: "100m", MemoryRequest: "64Mi",
		Workdir: "/work", Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	created, err := cs.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	got, err := cs.BatchV1().Jobs(ns).Get(ctx, created.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Labels["run-id"] != "contract-test" {
		t.Errorf("labels not persisted: %v", got.Labels)
	}

	fg := metav1.DeletePropagationForeground
	if err := cs.BatchV1().Jobs(ns).Delete(ctx, created.Name, metav1.DeleteOptions{PropagationPolicy: &fg}); err != nil {
		t.Fatalf("delete job: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err := cs.BatchV1().Jobs(ns).Get(ctx, created.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return
		}
		if err != nil && !apierrors.IsNotFound(err) {
			var statusErr *apierrors.StatusError
			if errors.As(err, &statusErr) {
				t.Fatalf("unexpected get error: %v", err)
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("job not deleted within deadline")
		}
		time.Sleep(time.Second)
	}
}
