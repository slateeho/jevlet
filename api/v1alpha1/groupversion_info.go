package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "jevlet.io", Version: "v1alpha1"}

var SchemeBuilder = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&JevPolicy{}, &JevPolicyList{},
		&JevIncident{}, &JevIncidentList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
})

var AddToScheme = SchemeBuilder.AddToScheme
