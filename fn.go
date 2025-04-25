package main

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/giantswarm/xfnlib/pkg/composite"

	"github.com/giantswarm/crossplane-fn-irsa/pkg/input/v1beta1"
)

const composedName = "crossplane-fn-irsa"

// RunFunction Execute the desired reconcilliation state, creating any required resources
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (rsp *fnv1beta1.RunFunctionResponse, err error) {
	f.log.Info("preparing function", composedName, req.GetMeta().GetTag())
	rsp = response.To(req, response.DefaultTTL)

	var (
		ac    XrConfig = XrConfig{}
		input v1beta1.Input
	)

	if ac.composed, err = composite.New(req, &input, &ac.composite); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "error setting up function "+composedName))
		return rsp, nil
	}

	if input.Spec == nil {
		response.Fatal(rsp, &composite.MissingSpec{})
		return rsp, nil
	}

	// FUNCTION BODY
	var region, providerConfigRef string
	region = "eu-central-1" // Default region, ideally this would come from input
	providerConfigRef = "aws-provider" // Default provider config, ideally this would come from input

	if err = f.DiscoverHostedZone(input.Spec.Domain, input.Spec.Tags, &region, &providerConfigRef, input.Spec.PatchTo, ac.composed); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot discover hosted zone for domain %q", input.Spec.Domain))
		return rsp, nil
	}

	if err = ac.composed.ToResponse(rsp); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot convert composition to response %T", rsp))
		return
	}

	return rsp, nil
}
