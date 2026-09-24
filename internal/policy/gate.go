package policy

import (
	"fmt"
	"math"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	jevletv1alpha1 "github.com/slateeho/jevlet/api/v1alpha1"
	"github.com/slateeho/jevlet/internal/jev"
)

type GateResult struct {
	Approved bool
	Reason   string
}

func Evaluate(p *jevletv1alpha1.JevPolicy, pod *corev1.Pod, d jev.Decision) GateResult {
	exec := p.Spec.Execution
	mode := strings.ToLower(strings.TrimSpace(exec.Mode))
	if mode == "" {
		mode = jevletv1alpha1.ExecutionModeObserve
	}
	if mode != jevletv1alpha1.ExecutionModeGuarded {
		return GateResult{Reason: "policy is not in guarded execution mode"}
	}
	if d.Action != jevletv1alpha1.ActionRestartPod {
		return GateResult{Reason: "selected action is non-mutating or requires human handling"}
	}
	if !slices.Contains(exec.AllowedActions, d.Action) {
		return GateResult{Reason: fmt.Sprintf("action %q is not allowlisted", d.Action)}
	}
	minConfidence := exec.MinimumConfidence
	if minConfidence == 0 {
		minConfidence = 0.92
	}
	if d.ActionConfidence < minConfidence {
		return GateResult{Reason: fmt.Sprintf("action confidence %.3f is below %.3f", d.ActionConfidence, minConfidence)}
	}
	minSafe := exec.MinimumSafeToExecute
	if minSafe == 0 {
		minSafe = 0.95
	}
	if d.SafeToExecute < minSafe {
		return GateResult{Reason: fmt.Sprintf("safe-to-execute probability %.3f is below %.3f", d.SafeToExecute, minSafe)}
	}
	maxRisk := exec.MaximumRiskScore
	if maxRisk == 0 {
		maxRisk = 1.5
	}
	if math.IsNaN(d.RiskScore) || d.RiskScore > maxRisk {
		return GateResult{Reason: fmt.Sprintf("risk score %.3f exceeds %.3f", d.RiskScore, maxRisk)}
	}
	if isSystemNamespace(pod.Namespace) {
		return GateResult{Reason: "automatic mutation is disabled in Kubernetes system namespaces"}
	}
	if controllerOwner(pod) == nil {
		return GateResult{Reason: "Pod has no controller owner; restart could delete an unmanaged Pod"}
	}
	return GateResult{Approved: true, Reason: "all deterministic execution gates passed"}
}

func controllerOwner(pod *corev1.Pod) *metav1.OwnerReference {
	for i := range pod.OwnerReferences {
		ref := &pod.OwnerReferences[i]
		if ref.Controller != nil && *ref.Controller {
			return ref
		}
	}
	return nil
}

func isSystemNamespace(ns string) bool {
	return ns == "kube-system" || ns == "kube-public" || ns == "kube-node-lease"
}
