package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
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

type CloudFrontApi interface {
	ListDistributions(ctx context.Context,
		params *cloudfront.ListDistributionsInput,
		optFns ...func(*cloudfront.Options)) (*cloudfront.ListDistributionsOutput, error)
}

type AwsStsApi interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

type IamApi interface {
	ListOpenIDConnectProviders(ctx context.Context,
		params *iam.ListOpenIDConnectProvidersInput,
		optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error)
	GetOpenIDConnectProvider(ctx context.Context,
		params *iam.GetOpenIDConnectProviderInput,
		optFns ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error)
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

	getIamClient = func(cfg aws.Config, ep string) IamApi {
		if ep != "" {
			return iam.NewFromConfig(cfg, func(o *iam.Options) {
				o.BaseEndpoint = &ep
			})
		}
		return iam.NewFromConfig(cfg)
	}

	getCloudFrontClient = func(cfg aws.Config, ep string) CloudFrontApi {
		if ep != "" {
			return cloudfront.NewFromConfig(cfg, func(o *cloudfront.Options) {
				o.BaseEndpoint = &ep
			})
		}
		return cloudfront.NewFromConfig(cfg)
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

func (f *Function) importDistribution(domain string, region string, providerConfigRef string, composed *composite.Composition) (err error) {
	var (
		cfg      aws.Config
		services map[string]string
		client   CloudFrontApi
	)

	f.log.Debug("Importing CloudFront distribution", "domain", domain)

	if cfg, services, err = awsConfig(&region, &providerConfigRef, f.log); err != nil {
		f.log.Info("Error loading aws config", "error", err)
		err = errors.Wrap(err, "failed to load aws config")
		return err
	}

	var ep string
	var ok bool
	if _, ok = services["cloudfront"]; ok {
		ep = services["cloudfront"]
	}

	client = getCloudFrontClient(cfg, ep)

	var distributions *cloudfront.ListDistributionsOutput
	distributions, err = client.ListDistributions(context.Background(), &cloudfront.ListDistributionsInput{})
	if err != nil {
		f.log.Info("Error listing distributions", "error", err)
		return err
	}

	var matchingDistributions []cloudfronttypes.DistributionSummary
	for _, dist := range distributions.DistributionList.Items {
		for _, alias := range dist.Aliases.Items {
			if alias == domain {
				matchingDistributions = append(matchingDistributions, dist)
			}
		}
	}

	f.log.Debug("matching distributions", "matchingDistributions", matchingDistributions)

	if len(matchingDistributions) == 0 {
		f.log.Debug("No matching distribution found", "domain", domain)
		return nil
	}

	if len(matchingDistributions) > 1 {
		err = errors.New("multiple distributions found matching the domain: " + domain)
		return err
	}

	distributionId := *matchingDistributions[0].Id
	f.log.Debug("Found distribution", "distributionId", distributionId)

	err = f.patchFieldValueToObject("status.importResources.cloudfrontDistributionId", distributionId, composed.DesiredComposite.Resource)
	return err
}

func (f *Function) importOpenIdProvider(domain string, region string, providerConfigRef string, composed *composite.Composition) (err error) {
	var (
		cfg      aws.Config
		services map[string]string
		client   IamApi
	)

	f.log.Debug("Importing OpenID Connect provider", "domain", domain)

	if cfg, services, err = awsConfig(&region, &providerConfigRef, f.log); err != nil {
		f.log.Info("Error loading aws config", "error", err)
		err = errors.Wrap(err, "failed to load aws config")
		return err
	}

	var ep string
	var ok bool
	if _, ok = services["iam"]; ok {
		ep = services["iam"]
	}

	client = getIamClient(cfg, ep)

	var providers *iam.ListOpenIDConnectProvidersOutput
	providers, err = client.ListOpenIDConnectProviders(context.Background(), &iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		f.log.Info("Error listing OpenID Connect providers", "error", err)
		return err
	}

	var matchingProviderArn string
	for _, providerArn := range providers.OpenIDConnectProviderList {
		var provider *iam.GetOpenIDConnectProviderOutput
		provider, err = client.GetOpenIDConnectProvider(context.Background(), &iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: providerArn.Arn,
		})
		if err != nil {
			f.log.Info("Error getting OpenID Connect provider details", "arn", *providerArn.Arn, "error", err)
			continue
		}

		if *provider.Url == fmt.Sprintf("https://%s", domain) {
			matchingProviderArn = *providerArn.Arn
			break
		}
	}

	f.log.Debug("matching provider", "arn", matchingProviderArn)

	if matchingProviderArn == "" {
		f.log.Debug("No matching provider found", "domain", domain)
		return nil
	}

	err = f.patchFieldValueToObject("status.importResources.openIdProviderArn", matchingProviderArn, composed.DesiredComposite.Resource)
	return err
}
