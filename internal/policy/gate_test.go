package policy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	jevletv1alpha1 "github.com/slateeho/jevlet/api/v1alpha1"
	"github.com/slateeho/jevlet/internal/jev"
)

func TestGuardedRestart(t *testing.T) {
	controller := true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "app", Name: "backend-x", UID: types.UID("pod-1"),
		OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "backend-abc", UID: types.UID("rs-1"), Controller: &controller}},
	}}
	p := &jevletv1alpha1.JevPolicy{Spec: jevletv1alpha1.JevPolicySpec{Execution: jevletv1alpha1.ExecutionSpec{
		Mode:                 jevletv1alpha1.ExecutionModeGuarded,
		AllowedActions:       []string{jevletv1alpha1.ActionRestartPod},
		MinimumConfidence:    0.9,
		MinimumSafeToExecute: 0.9,
		MaximumRiskScore:     2,
	}}}
	d := jev.Decision{Action: jevletv1alpha1.ActionRestartPod, ActionConfidence: 0.96, SafeToExecute: 0.97, RiskScore: 1}
	got := Evaluate(p, pod, d)
	if !got.Approved {
		t.Fatalf("expected approval, got %q", got.Reason)
	}
}

func TestSystemNamespaceDenied(t *testing.T) {
	controller := true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", OwnerReferences: []metav1.OwnerReference{{Controller: &controller}}}}
	p := &jevletv1alpha1.JevPolicy{Spec: jevletv1alpha1.JevPolicySpec{Execution: jevletv1alpha1.ExecutionSpec{Mode: "guarded", AllowedActions: []string{"restart_pod"}}}}
	d := jev.Decision{Action: "restart_pod", ActionConfidence: 1, SafeToExecute: 1, RiskScore: 0}
	if got := Evaluate(p, pod, d); got.Approved {
		t.Fatal("system namespace must be denied")
	}
}
