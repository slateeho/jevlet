// Code generated manually for the initial bootstrap; DO NOT EDIT casually.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (in *JevPolicy) DeepCopyInto(out *JevPolicy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Spec.Selector.DeepCopyInto(&out.Spec.Selector)
	if in.Spec.Signals.Prometheus != nil {
		out.Spec.Signals.Prometheus = &PrometheusSpec{URL: in.Spec.Signals.Prometheus.URL}
		if in.Spec.Signals.Prometheus.Queries != nil {
			out.Spec.Signals.Prometheus.Queries = make(map[string]string, len(in.Spec.Signals.Prometheus.Queries))
			for k, v := range in.Spec.Signals.Prometheus.Queries {
				out.Spec.Signals.Prometheus.Queries[k] = v
			}
		}
	}
	if in.Spec.Execution.AllowedActions != nil {
		out.Spec.Execution.AllowedActions = append([]string(nil), in.Spec.Execution.AllowedActions...)
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = append([]metav1.Condition(nil), in.Status.Conditions...)
	}
}

func (in *JevPolicy) DeepCopy() *JevPolicy {
	if in == nil {
		return nil
	}
	out := new(JevPolicy)
	in.DeepCopyInto(out)
	return out
}
func (in *JevPolicy) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *JevPolicyList) DeepCopyInto(out *JevPolicyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]JevPolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
func (in *JevPolicyList) DeepCopy() *JevPolicyList {
	if in == nil {
		return nil
	}
	out := new(JevPolicyList)
	in.DeepCopyInto(out)
	return out
}
func (in *JevPolicyList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *JevIncident) DeepCopyInto(out *JevIncident) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
	if in.Status.DecidedAt != nil {
		t := in.Status.DecidedAt.DeepCopy()
		out.Status.DecidedAt = t
	}
}
func (in *JevIncident) DeepCopy() *JevIncident {
	if in == nil {
		return nil
	}
	out := new(JevIncident)
	in.DeepCopyInto(out)
	return out
}
func (in *JevIncident) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *JevIncidentList) DeepCopyInto(out *JevIncidentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]JevIncident, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
func (in *JevIncidentList) DeepCopy() *JevIncidentList {
	if in == nil {
		return nil
	}
	out := new(JevIncidentList)
	in.DeepCopyInto(out)
	return out
}
func (in *JevIncidentList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
