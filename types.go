package main

import (
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/giantswarm/xfnlib/pkg/composite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EksImportXRObject is the information we are going to pull from the XR
type XRObject struct {
	Metadata metav1.ObjectMeta `json:"metadata"`
	Spec     XRSpec            `json:"spec"`
}

type XRSpec struct {
	Labels            map[string]string `json:"labels"`
	ProviderConfigRef string            `json:"providerConfigRef"`
	DeletionPolicy    string            `json:"deletionPolicy"`
	ClaimRef          struct {
		Namespace string `json:"namespace"`
	} `json:"claimRef"`

	CompositionSelector struct {
		MatchLabels struct {
			Provider string `json:"provider"`
		} `json:"matchLabels"`
	} `json:"compositionSelector"`
}

// Function returns whatever response you ask it to.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer
	log       logging.Logger
	composed  *composite.Composition
	composite XRObject
}

type XrConfig struct {
	cluster, namespace, region, providerConfigRef *string
	labels, annotations                           map[string]string
	composed                                      *composite.Composition
	composite                                     EksImportXRObject
}

// EksImportXRObject is the information we are going to pull from the XR
type EksImportXRObject struct {
	Metadata metav1.ObjectMeta `json:"metadata"`
	Spec     any               `json:"spec"`
}
