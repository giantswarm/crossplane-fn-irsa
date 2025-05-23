// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=irsa.fn.giantswarm.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// Input can be used to provide input to this Function.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Defines the spec for this input
	Spec *Spec `json:"spec,omitempty"`
}

// Spec - Defines the spec given to this input type, providing the required, and optional elements that may be defined
type Spec struct {
	// +optional
	DomainRef string `json:"domainRef"`

	// +optional
	Route53HostedZonePatchToRef string `json:"route53HostedZonePatchToRef"`

	// +required
	S3KeysPatchToRef string `json:"s3KeysPatchToRef"`

	// +required
	S3BucketNameRef string `json:"s3BucketNameRef"`

	// +required
	S3DiscoveryPatchToRef string `json:"s3DiscoveryPatchToRef"`

	// +required
	RegionRef string `json:"regionRef"`

	// +required
	ProviderConfigRef string `json:"providerConfigRef"`
}
