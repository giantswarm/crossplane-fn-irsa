package main

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	xfnaws "github.com/giantswarm/xfnlib/pkg/auth/aws"
	"github.com/giantswarm/xfnlib/pkg/composite"
)

type Route53Api interface {
	ListHostedZones(ctx context.Context,
		params *route53.ListHostedZonesInput,
		optFns ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error)
	ListTagsForResource(ctx context.Context,
		params *route53.ListTagsForResourceInput,
		optFns ...func(*route53.Options)) (*route53.ListTagsForResourceOutput, error)
}

type AwsStsApi interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

func GetHostedZones(c context.Context, api Route53Api, input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	return api.ListHostedZones(c, input)
}

func GetTagsForResource(c context.Context, api Route53Api, input *route53.ListTagsForResourceInput) (*route53.ListTagsForResourceOutput, error) {
	return api.ListTagsForResource(c, input)
}

func GetCallerIdentity(c context.Context, api AwsStsApi, input *sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
	return api.GetCallerIdentity(c, input)
}

var (
	getRoute53Client = func(cfg aws.Config, ep string) Route53Api {
		if ep != "" {
			return route53.NewFromConfig(cfg, func(o *route53.Options) {
				o.BaseEndpoint = &ep
			})
		}
		return route53.NewFromConfig(cfg)
	}

	getStsClient = func(cfg aws.Config, ep string) AwsStsApi {
		if ep != "" {
			return sts.NewFromConfig(cfg, func(o *sts.Options) {
				o.BaseEndpoint = &ep
			})
		}
		return sts.NewFromConfig(cfg)
	}

	awsConfig = func(region, providerCfg *string, log logging.Logger) (aws.Config, map[string]string, error) {
		return xfnaws.Config(region, providerCfg, log)
	}
)

func (f *Function) GetAccountId(region, pcr *string) (id string, err error) {
	var (
		cfg       aws.Config
		services  map[string]string
		stsclient AwsStsApi
	)

	if cfg, services, err = awsConfig(region, pcr, f.log); err != nil {
		err = errors.Wrap(err, "failed to load aws config")
		return
	}

	var ep string
	var ok bool
	if _, ok = services["sts"]; ok {
		ep = services["sts"]
	}

	stsclient = getStsClient(cfg, ep)
	var identity *sts.GetCallerIdentityOutput
	{
		identity, err = GetCallerIdentity(context.Background(), stsclient, &sts.GetCallerIdentityInput{})
		if err != nil {
			return
		}
	}

	id = *identity.Account
	return
}

func (f *Function) DiscoverHostedZone(domain string, region string, providerConfigRef string, patchTo string, composed *composite.Composition) (err error) {
	var (
		cfg      aws.Config
		services map[string]string
		client   Route53Api
	)

	f.log.Debug("Discovering hosted zone", "domain", domain)

	if cfg, services, err = awsConfig(&region, &providerConfigRef, f.log); err != nil {
		f.log.Info("Error loading aws config", "error", err)
		err = errors.Wrap(err, "failed to load aws config with region "+region)
		return err
	}

	var ep string
	var ok bool
	if _, ok = services["route53"]; ok {
		ep = services["route53"]
	}

	client = getRoute53Client(cfg, ep)

	var hostedZones *route53.ListHostedZonesOutput
	hostedZones, err = GetHostedZones(context.Background(), client, &route53.ListHostedZonesInput{})
	if err != nil {
		f.log.Info("Error listing hosted zones", "error", err)
		return err
	}

	var matchingHostedZones []route53types.HostedZone
	for _, hz := range hostedZones.HostedZones {
		zoneName := strings.TrimSuffix(*hz.Name, ".")
		if zoneName == domain {
			matchingHostedZones = append(matchingHostedZones, hz)
		}
	}

	f.log.Debug("matching hosted zones", "matchingHostedZones", matchingHostedZones)

	if len(matchingHostedZones) == 0 {
		err = errors.New("no hosted zone found matching the domain: " + domain)
		return err
	}

	if len(matchingHostedZones) > 1 {
		err = errors.New("multiple hosted zones found matching the domain: " + domain)
		return err
	}

	hostedZoneId := strings.TrimPrefix(*matchingHostedZones[0].Id, "/hostedzone/")
	f.log.Debug("Found hosted zone", "hostedZoneId", hostedZoneId)

	err = f.patchFieldValueToObject(patchTo, hostedZoneId, composed.DesiredComposite.Resource)
	return err
}
