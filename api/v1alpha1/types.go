package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	ExecutionModeObserve = "observe"
	ExecutionModeGuarded = "guarded"

	ActionNoop       = "noop"
	ActionEscalate   = "escalate"
	ActionRestartPod = "restart_pod"
)

type PrometheusSpec struct {
	URL     string            `json:"url"`
	Queries map[string]string `json:"queries,omitempty"`
}

type SignalsSpec struct {
	KubernetesEvents bool            `json:"kubernetesEvents,omitempty"`
	MaxEvents        int32           `json:"maxEvents,omitempty"`
	Prometheus       *PrometheusSpec `json:"prometheus,omitempty"`
}

type DecisionSpec struct {
	Model string `json:"model,omitempty"`
}

type ExecutionSpec struct {
	Mode                 string          `json:"mode,omitempty"`
	AllowedActions       []string        `json:"allowedActions,omitempty"`
	MinimumConfidence    float64         `json:"minimumConfidence,omitempty"`
	MinimumSafeToExecute float64         `json:"minimumSafeToExecute,omitempty"`
	MaximumRiskScore     float64         `json:"maximumRiskScore,omitempty"`
	Cooldown             metav1.Duration `json:"cooldown,omitempty"`
}

type JevPolicySpec struct {
	Selector  metav1.LabelSelector `json:"selector"`
	Signals   SignalsSpec          `json:"signals,omitempty"`
	Decision  DecisionSpec         `json:"decision,omitempty"`
	Execution ExecutionSpec        `json:"execution,omitempty"`
}

type JevPolicyStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// JevPolicy declares which Pods Jevlet may observe and which closed actions may be executed.
type JevPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              JevPolicySpec   `json:"spec,omitempty"`
	Status            JevPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type JevPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []JevPolicy `json:"items"`
}

type TargetRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
	OwnerKind  string `json:"ownerKind,omitempty"`
	OwnerName  string `json:"ownerName,omitempty"`
	OwnerUID   string `json:"ownerUID,omitempty"`
}

type JevIncidentSpec struct {
	PolicyName  string    `json:"policyName"`
	Target      TargetRef `json:"target"`
	IssueReason string    `json:"issueReason"`
	StateDigest string    `json:"stateDigest"`
}

type DecisionReceipt struct {
	RequestedModel   string  `json:"requestedModel,omitempty"`
	ResolvedModel    string  `json:"resolvedModel,omitempty"`
	ProbableCause    string  `json:"probableCause,omitempty"`
	CauseConfidence  float64 `json:"causeConfidence,omitempty"`
	Action           string  `json:"action,omitempty"`
	ActionConfidence float64 `json:"actionConfidence,omitempty"`
	RiskScore        float64 `json:"riskScore,omitempty"`
	RiskConfidence   float64 `json:"riskConfidence,omitempty"`
	SafeToExecute    float64 `json:"safeToExecute,omitempty"`
	InputTokens      int     `json:"inputTokens,omitempty"`
	OutputTokens     int     `json:"outputTokens,omitempty"`
	Approved         bool    `json:"approved,omitempty"`
	ApprovalReason   string  `json:"approvalReason,omitempty"`
	Executed         bool    `json:"executed,omitempty"`
	ExecutionResult  string  `json:"executionResult,omitempty"`
}

type JevIncidentStatus struct {
	Phase     string          `json:"phase,omitempty"`
	Decision  DecisionReceipt `json:"decision,omitempty"`
	DecidedAt *metav1.Time    `json:"decidedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// JevIncident is an immutable-ish decision receipt linked to a workload observation.
type JevIncident struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              JevIncidentSpec   `json:"spec,omitempty"`
	Status            JevIncidentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type JevIncidentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []JevIncident `json:"items"`
}
