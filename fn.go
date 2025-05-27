package main

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"

	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/giantswarm/xfnlib/pkg/composite"

	"github.com/giantswarm/crossplane-fn-irsa/pkg/input/v1beta1"
)

const composedName = "crossplane-fn-irsa"

// RunFunction Execute the desired reconcilliation state, creating any required resources
func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (rsp *fnv1.RunFunctionResponse, err error) {
	rsp = response.To(req, response.DefaultTTL)

	var (
		composed       *composite.Composition
		input          v1beta1.Input
		region         string
		providerConfig string
		domain         string
		irsaDomain     string
		S3BucketName   string
	)

	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	if composed, err = composite.New(req, &input, &oxr); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "error setting up function "+composedName))
		return rsp, nil
	}

	if input.Spec == nil {
		response.Fatal(rsp, &composite.MissingSpec{})
		return rsp, nil
	}

	// Extract region and provider config from input
	if region, err = f.getStringFromPaved(oxr.Resource, input.Spec.RegionRef); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get region from %q", input.Spec.RegionRef))
		return rsp, nil
	}
	f.log.Debug("Region", "region", region)

	if S3BucketName, err = f.getStringFromPaved(oxr.Resource, input.Spec.S3BucketNameRef); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get S3 bucket name from %q", input.Spec.S3BucketNameRef))
		return rsp, nil
	}
	f.log.Debug("S3BucketName", "S3BucketName", S3BucketName)

	if providerConfig, err = f.getStringFromPaved(oxr.Resource, input.Spec.ProviderConfigRef); err != nil {
		f.log.Info("cannot get provider config reference from input", "error", err)
		response.Fatal(rsp, errors.Wrap(err, "cannot get provider config reference from input"))
		return rsp, nil
	}
	f.log.Debug("ProviderConfig", "providerConfig", providerConfig)

	if input.Spec.DomainRef != "" {
		if domain, err = f.getStringFromPaved(oxr.Resource, input.Spec.DomainRef); err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot get region from %q", input.Spec.RegionRef))
			return rsp, nil
		}
		f.log.Debug("Domain", "domain", domain)

		irsaDomain = "irsa." + domain

		if err = f.DiscoverHostedZone(domain, region, providerConfig, input.Spec.Route53HostedZonePatchToRef, composed); err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot discover hosted zone for domain %q", domain))
			return rsp, nil
		}
	}

	if err = f.GenerateDiscoveryFile(irsaDomain, S3BucketName, region, input.Spec.S3DiscoveryPatchToRef, composed); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot generate discovery file for domain %q", domain))
		return rsp, nil
	}

	key, err := f.ServiceAccountSecret(oxr.Resource.GetLabels()["crossplane.io/claim-namespace"], oxr.Resource.GetLabels()["crossplane.io/claim-name"])
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get service account secret"))
		return rsp, nil
	}

	if err = f.GenerateKeysFile(key, input.Spec.S3KeysPatchToRef, composed); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot generate keys file for domain %q", domain))
		return rsp, nil
	}

	if err = f.importDistribution(irsaDomain, region, providerConfig, composed); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot generate import resources for domain %q", domain))
		return rsp, nil
	}

	if err = f.importOpenIdProvider(irsaDomain, region, providerConfig, composed); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot generate import resources for domain %q", domain))
		return rsp, nil
	}

	if err = composed.ToResponse(rsp); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot convert composition to response %T", rsp))
		return
	}

	return rsp, nil
}

func (f *Function) getStringFromPaved(req runtime.Object, ref string) (value string, err error) {
	var paved *fieldpath.Paved
	if paved, err = fieldpath.PaveObject(req); err != nil {
		return
	}

	value, err = paved.GetString(ref)
	return
}

func (f *Function) patchFieldValueToObject(fieldPath string, value any, to runtime.Object) (err error) {
	var paved *fieldpath.Paved
	if paved, err = fieldpath.PaveObject(to); err != nil {
		return
	}

	if err = paved.SetValue(fieldPath, value); err != nil {
		return
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(paved.UnstructuredContent(), to)
}
