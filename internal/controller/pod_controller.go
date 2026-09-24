package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	jevletv1alpha1 "github.com/slateeho/jevlet/api/v1alpha1"
	"github.com/slateeho/jevlet/internal/jev"
	"github.com/slateeho/jevlet/internal/policy"
	"github.com/slateeho/jevlet/internal/signals"
)

var (
	decisionsTotal  = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "jevlet_decisions_total", Help: "Jev decisions by action and approval."}, []string{"action", "approved"})
	decisionLatency = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "jevlet_decision_latency_seconds", Help: "TypeSafe Jev decision latency in seconds.", Buckets: prometheus.DefBuckets})
	apiErrorsTotal  = prometheus.NewCounter(prometheus.CounterOpts{Name: "jevlet_api_errors_total", Help: "TypeSafe API errors."})
	actionsTotal    = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "jevlet_actions_total", Help: "Attempted Jevlet actions by action and result."}, []string{"action", "result"})
)

func init() {
	crmetrics.Registry.MustRegister(decisionsTotal, decisionLatency, apiErrorsTotal, actionsTotal)
}

type PodReconciler struct {
	client.Client
	APIReader client.Reader
	Jev       *jev.Client
	Prom      signals.PrometheusClient
	Now       func() time.Time
}

type podIssue struct {
	Reason, Message string
	RestartCount    int32
}

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("pod", req.NamespacedName)
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if pod.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	issue, ok := detectIssue(&pod)
	if !ok {
		return ctrl.Result{}, nil
	}

	var policies jevletv1alpha1.JevPolicyList
	if err := r.List(ctx, &policies, client.InNamespace(pod.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	for i := range policies.Items {
		p := &policies.Items[i]
		selector, err := metav1.LabelSelectorAsSelector(&p.Spec.Selector)
		if err != nil {
			logger.Error(err, "invalid policy selector", "policy", p.Name)
			continue
		}
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		if err := r.evaluatePolicy(ctx, &pod, p, issue); err != nil {
			logger.Error(err, "policy evaluation failed", "policy", p.Name)
		}
	}
	return ctrl.Result{}, nil
}

func (r *PodReconciler) evaluatePolicy(ctx context.Context, pod *corev1.Pod, p *jevletv1alpha1.JevPolicy, issue podIssue) error {

	if blocked, _, err := r.inCooldown(ctx, pod, p); err != nil {
		return err
	} else if blocked {
		return nil // Pod updates during cooldown must not burn additional model calls.
	}

	state := map[string]any{
		"kubernetes": map[string]any{
			"namespace":    pod.Namespace,
			"pod":          pod.Name,
			"phase":        pod.Status.Phase,
			"reason":       issue.Reason,
			"message":      truncate(issue.Message, 2048),
			"restartCount": issue.RestartCount,
			"nodeName":     pod.Spec.NodeName,
			"qosClass":     pod.Status.QOSClass,
			"owner":        ownerState(pod),
		},
	}
	if p.Spec.Signals.KubernetesEvents {
		state["events"] = r.events(ctx, pod, p.Spec.Signals.MaxEvents)
	}
	if ps := p.Spec.Signals.Prometheus; ps != nil && strings.TrimSpace(ps.URL) != "" {
		metrics := map[string]any{}
		keys := make([]string, 0, len(ps.Queries))
		for k := range ps.Queries {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, name := range keys {
			query := signals.RenderQuery(ps.Queries[name], pod.Namespace, pod.Name)
			v, err := r.Prom.Query(ctx, ps.URL, query)
			if err != nil {
				metrics[name] = map[string]any{"error": err.Error()}
				continue
			}
			metrics[name] = v
		}
		state["prometheus"] = metrics
	}

	digest, err := stateDigest(state)
	if err != nil {
		return err
	}
	model := p.Spec.Decision.Model
	decision, err := r.Jev.Decide(ctx, state, model)
	if err != nil {
		apiErrorsTotal.Inc()
		return err
	}
	decisionLatency.Observe(decision.Latency.Seconds())
	gate := policy.Evaluate(p, pod, decision)

	incident := &jevletv1alpha1.JevIncident{
		TypeMeta:   metav1.TypeMeta{APIVersion: jevletv1alpha1.GroupVersion.String(), Kind: "JevIncident"},
		ObjectMeta: metav1.ObjectMeta{GenerateName: "jev-", Namespace: pod.Namespace, Labels: map[string]string{"jevlet.io/policy": p.Name}},
		Spec:       jevletv1alpha1.JevIncidentSpec{PolicyName: p.Name, Target: targetRef(pod), IssueReason: issue.Reason, StateDigest: digest},
	}
	if err := r.Create(ctx, incident); err != nil {
		return fmt.Errorf("create incident: %w", err)
	}
	now := metav1.NewTime(r.now().UTC())
	incident.Status = jevletv1alpha1.JevIncidentStatus{Phase: "Decided", DecidedAt: &now, Decision: jevletv1alpha1.DecisionReceipt{
		RequestedModel: decision.RequestedModel, ResolvedModel: decision.ResolvedModel,
		ProbableCause: decision.ProbableCause, CauseConfidence: decision.CauseConfidence,
		Action: decision.Action, ActionConfidence: decision.ActionConfidence,
		RiskScore: decision.RiskScore, RiskConfidence: decision.RiskConfidence,
		SafeToExecute: decision.SafeToExecute, InputTokens: decision.Usage.InputTokens, OutputTokens: decision.Usage.OutputTokens,
		Approved: gate.Approved, ApprovalReason: gate.Reason,
	}}
	approvedLabel := strconv.FormatBool(gate.Approved)
	decisionsTotal.WithLabelValues(decision.Action, approvedLabel).Inc()

	if gate.Approved && decision.Action == jevletv1alpha1.ActionRestartPod {
		uid := pod.UID
		err := r.Delete(ctx, pod, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
		if err != nil {
			incident.Status.Phase = "ActionFailed"
			incident.Status.Decision.ExecutionResult = err.Error()
			actionsTotal.WithLabelValues(decision.Action, "error").Inc()
		} else {
			incident.Status.Phase = "Executed"
			incident.Status.Decision.Executed = true
			incident.Status.Decision.ExecutionResult = "pod deletion accepted with UID precondition"
			actionsTotal.WithLabelValues(decision.Action, "success").Inc()
		}
	}
	if err := r.Status().Update(ctx, incident); err != nil {
		return fmt.Errorf("update incident status: %w", err)
	}
	return nil
}

func (r *PodReconciler) inCooldown(ctx context.Context, pod *corev1.Pod, p *jevletv1alpha1.JevPolicy) (bool, time.Duration, error) {
	cooldown := p.Spec.Execution.Cooldown.Duration
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}
	var list jevletv1alpha1.JevIncidentList
	if err := r.List(ctx, &list, client.InNamespace(pod.Namespace), client.MatchingLabels{"jevlet.io/policy": p.Name}); err != nil {
		return false, 0, err
	}
	owner := controllerOwner(pod)
	ownerUID := string(pod.UID)
	if owner != nil {
		ownerUID = string(owner.UID)
	}
	for i := range list.Items {
		it := &list.Items[i]
		if it.Status.DecidedAt == nil {
			continue
		}
		incOwnerUID := it.Spec.Target.OwnerUID
		if incOwnerUID == "" {
			incOwnerUID = it.Spec.Target.UID
		}
		if incOwnerUID != ownerUID {
			continue
		}
		age := r.now().Sub(it.Status.DecidedAt.Time)
		if age >= 0 && age < cooldown {
			return true, cooldown - age, nil
		}
	}
	return false, 0, nil
}

func (r *PodReconciler) events(ctx context.Context, pod *corev1.Pod, max int32) []map[string]any {
	if max <= 0 || max > 20 {
		max = 8
	}
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	var list corev1.EventList
	opts := &client.ListOptions{Namespace: pod.Namespace, Raw: &metav1.ListOptions{FieldSelector: "involvedObject.uid=" + string(pod.UID)}}
	if err := reader.List(ctx, &list, opts); err != nil {
		return []map[string]any{{"error": err.Error()}}
	}
	sort.Slice(list.Items, func(i, j int) bool { return eventTime(list.Items[i]).After(eventTime(list.Items[j])) })
	if len(list.Items) > int(max) {
		list.Items = list.Items[:max]
	}
	out := make([]map[string]any, 0, len(list.Items))
	for _, e := range list.Items {
		out = append(out, map[string]any{"type": e.Type, "reason": e.Reason, "message": truncate(e.Message, 1024), "count": e.Count, "time": eventTime(e).UTC().Format(time.RFC3339)})
	}
	return out
}

func eventTime(e corev1.Event) time.Time {
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	return e.CreationTimestamp.Time
}

func detectIssue(pod *corev1.Pod) (podIssue, bool) {
	var restarts int32
	for _, s := range pod.Status.InitContainerStatuses {
		restarts += s.RestartCount
		if w := s.State.Waiting; w != nil && actionableWaitingReason(w.Reason) {
			return podIssue{w.Reason, w.Message, restarts}, true
		}
	}
	for _, s := range pod.Status.ContainerStatuses {
		restarts += s.RestartCount
		if w := s.State.Waiting; w != nil && actionableWaitingReason(w.Reason) {
			return podIssue{w.Reason, w.Message, restarts}, true
		}
		if t := s.State.Terminated; t != nil && t.ExitCode != 0 {
			return podIssue{Reason: "ContainerTerminated", Message: t.Message, RestartCount: restarts}, true
		}
	}
	if restarts >= 3 {
		return podIssue{Reason: "RepeatedRestarts", Message: "container restart count is elevated", RestartCount: restarts}, true
	}
	if pod.Status.Phase == corev1.PodFailed {
		return podIssue{Reason: string(pod.Status.Reason), Message: pod.Status.Message, RestartCount: restarts}, true
	}
	return podIssue{}, false
}

func actionableWaitingReason(reason string) bool {
	switch reason {
	case "CrashLoopBackOff", "CreateContainerConfigError", "CreateContainerError", "ImagePullBackOff", "ErrImagePull", "RunContainerError", "InvalidImageName":
		return true
	default:
		return false
	}
}

func ownerState(pod *corev1.Pod) map[string]any {
	if o := controllerOwner(pod); o != nil {
		return map[string]any{"kind": o.Kind, "name": o.Name, "uid": string(o.UID)}
	}
	return map[string]any{"kind": "Pod", "name": pod.Name, "uid": string(pod.UID)}
}
func controllerOwner(pod *corev1.Pod) *metav1.OwnerReference {
	for i := range pod.OwnerReferences {
		r := &pod.OwnerReferences[i]
		if r.Controller != nil && *r.Controller {
			return r
		}
	}
	return nil
}
func targetRef(pod *corev1.Pod) jevletv1alpha1.TargetRef {
	t := jevletv1alpha1.TargetRef{APIVersion: "v1", Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, UID: string(pod.UID)}
	if o := controllerOwner(pod); o != nil {
		t.OwnerKind = o.Kind
		t.OwnerName = o.Name
		t.OwnerUID = string(o.UID)
	}
	return t
}
func stateDigest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (r *PodReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&corev1.Pod{}).Named("pod-incident").Complete(r)
}
