package main

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
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

// acmCertificateRegion is the region ACM certificates must live in for
// CloudFront distributions. CloudFront only accepts viewer certificates from
// us-east-1, regardless of where the distribution's other resources are.
const acmCertificateRegion = "us-east-1"

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

type AcmApi interface {
	ListCertificates(ctx context.Context,
		params *acm.ListCertificatesInput,
		optFns ...func(*acm.Options)) (*acm.ListCertificatesOutput, error)
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

	getAcmClient = func(cfg aws.Config, ep string) AcmApi {
		if ep != "" {
			return acm.NewFromConfig(cfg, func(o *acm.Options) {
				o.BaseEndpoint = &ep
			})
		}
		return acm.NewFromConfig(cfg)
	}

	awsConfig = func(region, providerCfgRef *string, log logging.Logger) (aws.Config, map[string]string, error) {
		awsCfg, services, err := xfnaws.Config(region, providerCfgRef, log)
		if err != nil {
			return aws.Config{}, nil, err
		}

		awsCfg.AppID = "crossplane-fn-irsa"
		return awsCfg, services, err
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
	if v, ok := services["sts"]; ok {
		ep = v
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
	if v, ok := services["route53"]; ok {
		ep = v
	}

	client = getRoute53Client(cfg, ep)

	var hostedZones *route53.ListHostedZonesOutput
	hostedZones, err = GetHostedZones(context.Background(), client, &route53.ListHostedZonesInput{})
	if err != nil {
		f.log.Info("Error listing hosted zones", "error", err)
		return err
	}

	// Fetch all hosted zones by paginating through results
	var allHostedZones []route53types.HostedZone
	allHostedZones = append(allHostedZones, hostedZones.HostedZones...)

	// Continue fetching if there are more hosted zones
	for hostedZones.IsTruncated {
		var nextMarker *string
		if hostedZones.NextMarker != nil {
			nextMarker = hostedZones.NextMarker
		} else if len(hostedZones.HostedZones) > 0 {
			// If NextMarker is not provided, use the ID of the last hosted zone
			nextMarker = hostedZones.HostedZones[len(hostedZones.HostedZones)-1].Id
		}

		hostedZones, err = GetHostedZones(context.Background(), client, &route53.ListHostedZonesInput{
			Marker:   nextMarker,
			MaxItems: aws.Int32(100),
		})
		if err != nil {
			f.log.Info("Error listing additional hosted zones", "error", err)
			return err
		}

		allHostedZones = append(allHostedZones, hostedZones.HostedZones...)
	}

	f.log.Debug("Total hosted zones found", "count", len(allHostedZones))

	var matchingHostedZones []route53types.HostedZone
	for _, hz := range allHostedZones {
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

func (f *Function) DiscoverDistribution(domain string, region string, providerConfigRef string, composed *composite.Composition) (err error) {
	var (
		cfg      aws.Config
		services map[string]string
		client   CloudFrontApi
	)

	f.log.Debug("Discovering CloudFront distribution", "domain", domain, "region", region)

	if cfg, services, err = awsConfig(&region, &providerConfigRef, f.log); err != nil {
		f.log.Info("Failed to load AWS config", "error", err, "region", region)
		err = errors.Wrap(err, "failed to load aws config")
		return err
	}

	var ep string
	if v, ok := services["cloudfront"]; ok {
		ep = v
		f.log.Debug("Using custom CloudFront endpoint", "endpoint", ep)
	}

	client = getCloudFrontClient(cfg, ep)

	var allDistributions []cloudfronttypes.DistributionSummary
	var marker *string
	for {
		distributions, lerr := client.ListDistributions(context.Background(), &cloudfront.ListDistributionsInput{
			Marker: marker,
		})
		if lerr != nil {
			f.log.Info("Failed to list CloudFront distributions", "error", lerr)
			return lerr
		}

		if distributions.DistributionList == nil {
			break
		}

		allDistributions = append(allDistributions, distributions.DistributionList.Items...)

		if distributions.DistributionList.IsTruncated == nil || !*distributions.DistributionList.IsTruncated {
			break
		}
		if distributions.DistributionList.NextMarker == nil || *distributions.DistributionList.NextMarker == "" {
			break
		}
		marker = distributions.DistributionList.NextMarker
	}

	f.log.Debug("Found distributions", "count", len(allDistributions))

	var matchingDistributions []cloudfronttypes.DistributionSummary
	for _, dist := range allDistributions {
		if dist.Aliases == nil {
			continue
		}
		for _, alias := range dist.Aliases.Items {
			if strings.EqualFold(alias, domain) {
				matchingDistributions = append(matchingDistributions, dist)
				f.log.Debug("Found matching distribution", "distributionId", dist.Id, "alias", alias)
			}
		}
	}

	if len(matchingDistributions) == 0 {
		f.log.Debug("No matching distribution found", "domain", domain)
		return nil
	}

	if len(matchingDistributions) > 1 {
		err = errors.New("multiple distributions found matching the domain: " + domain)
		f.log.Info("Multiple matching distributions found", "error", err, "domain", domain, "count", len(matchingDistributions))
		return err
	}

	matched := matchingDistributions[0]
	distributionId := matched.Id
	f.log.Info("Found matching distribution", "distributionId", distributionId, "domain", domain)

	err = f.patchFieldValueToObject("status.importResources.cloudfrontDistributionId", distributionId, composed.DesiredComposite.Resource)
	if err != nil {
		f.log.Info("Failed to patch distribution ID", "error", err, "distributionId", distributionId)
		return err
	}

	// Extract the OAI ID from the matched distribution's S3 origin so the OAI
	// managed resource can adopt it. The live distribution is the authoritative
	// source for which OAI is in use.
	if oaiId := extractOaiIdFromDistribution(matched); oaiId != "" {
		f.log.Info("Found OAI referenced by distribution", "oaiId", oaiId, "distributionId", distributionId)
		if err = f.patchFieldValueToObject("status.importResources.cloudfrontOaiId", oaiId, composed.DesiredComposite.Resource); err != nil {
			f.log.Info("Failed to patch OAI ID", "error", err, "oaiId", oaiId)
			return err
		}
	} else {
		f.log.Debug("Distribution has no S3 origin with OAI", "distributionId", distributionId)
	}

	return nil
}

// extractOaiIdFromDistribution returns the OAI ID referenced by the first S3
// origin with a non-empty OriginAccessIdentity, or "" if none is found.
// CloudFront stores the reference as "origin-access-identity/cloudfront/<ID>";
// Crossplane's OriginAccessIdentity MR uses just <ID> as its external-name.
func extractOaiIdFromDistribution(dist cloudfronttypes.DistributionSummary) string {
	if dist.Origins == nil {
		return ""
	}
	for _, origin := range dist.Origins.Items {
		if origin.S3OriginConfig == nil || origin.S3OriginConfig.OriginAccessIdentity == nil {
			continue
		}
		path := *origin.S3OriginConfig.OriginAccessIdentity
		if path == "" {
			continue
		}
		idx := strings.LastIndex(path, "/")
		if idx < 0 || idx == len(path)-1 {
			continue
		}
		return path[idx+1:]
	}
	return ""
}

func (f *Function) DiscoverCertificate(domain string, providerConfigRef string, composed *composite.Composition) (err error) {
	var (
		cfg      aws.Config
		services map[string]string
		client   AcmApi
	)

	region := acmCertificateRegion
	f.log.Debug("Discovering ACM certificate", "domain", domain, "region", region)

	if cfg, services, err = awsConfig(&region, &providerConfigRef, f.log); err != nil {
		f.log.Info("Failed to load AWS config", "error", err, "region", region)
		err = errors.Wrap(err, "failed to load aws config")
		return err
	}

	var ep string
	if v, ok := services["acm"]; ok {
		ep = v
		f.log.Debug("Using custom ACM endpoint", "endpoint", ep)
	}

	client = getAcmClient(cfg, ep)

	var matching []acmtypes.CertificateSummary
	var nextToken *string
	for {
		out, lerr := client.ListCertificates(context.Background(), &acm.ListCertificatesInput{
			NextToken: nextToken,
		})
		if lerr != nil {
			f.log.Info("Failed to list ACM certificates", "error", lerr)
			return lerr
		}

		for _, cert := range out.CertificateSummaryList {
			if cert.DomainName == nil || !strings.EqualFold(*cert.DomainName, domain) {
				continue
			}
			if cert.Status == acmtypes.CertificateStatusFailed ||
				cert.Status == acmtypes.CertificateStatusValidationTimedOut {
				continue
			}
			matching = append(matching, cert)
		}

		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}

	if len(matching) == 0 {
		f.log.Debug("No matching ACM certificate found", "domain", domain)
		return nil
	}

	chosen := matching[0]
	if len(matching) > 1 {
		// Prefer an ISSUED certificate when multiple match. Warn so the operator
		// can clean up stragglers — this shouldn't happen in steady state.
		for _, cert := range matching {
			if cert.Status == acmtypes.CertificateStatusIssued {
				chosen = cert
				break
			}
		}
		f.log.Info("Multiple ACM certificates matched domain; preferring ISSUED",
			"domain", domain, "count", len(matching),
			"chosenArn", chosen.CertificateArn, "chosenStatus", string(chosen.Status))
	}

	if chosen.CertificateArn == nil || *chosen.CertificateArn == "" {
		f.log.Info("Matched ACM certificate has no ARN", "domain", domain)
		return nil
	}

	f.log.Info("Found matching ACM certificate", "arn", *chosen.CertificateArn, "domain", domain)

	if err = f.patchFieldValueToObject("status.importResources.certificateArn", *chosen.CertificateArn, composed.DesiredComposite.Resource); err != nil {
		f.log.Info("Failed to patch certificate ARN", "error", err, "arn", *chosen.CertificateArn)
		return err
	}

	return nil
}

func (f *Function) DiscoverOpenIdProvider(domain string, region string, providerConfigRef string, composed *composite.Composition) (err error) {
	var (
		cfg      aws.Config
		services map[string]string
		client   IamApi
	)

	f.log.Debug("Discovering OpenID Connect provider", "domain", domain, "region", region)

	if cfg, services, err = awsConfig(&region, &providerConfigRef, f.log); err != nil {
		f.log.Info("Failed to load AWS config", "error", err, "region", region)
		err = errors.Wrap(err, "failed to load aws config")
		return err
	}

	var ep string
	if v, ok := services["iam"]; ok {
		ep = v
		f.log.Debug("Using custom IAM endpoint", "endpoint", ep)
	}

	client = getIamClient(cfg, ep)

	var providers *iam.ListOpenIDConnectProvidersOutput
	providers, err = client.ListOpenIDConnectProviders(context.Background(), &iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		f.log.Info("Failed to list OpenID Connect providers", "error", err)
		return err
	}

	f.log.Debug("Found OpenID Connect providers", "count", len(providers.OpenIDConnectProviderList))

	var matchingProviderArn string
	for _, providerArn := range providers.OpenIDConnectProviderList {
		if strings.Contains(*providerArn.Arn, domain) {
			matchingProviderArn = *providerArn.Arn
			f.log.Debug("Found matching provider", "arn", matchingProviderArn)
			break
		}
	}

	if matchingProviderArn == "" {
		f.log.Debug("No matching provider found", "domain", domain)
		return nil
	}

	f.log.Info("Found matching OpenID Connect provider", "arn", matchingProviderArn, "domain", domain)

	err = f.patchFieldValueToObject("status.importResources.openIdProviderArn", matchingProviderArn, composed.DesiredComposite.Resource)
	if err != nil {
		f.log.Info("Failed to patch provider ARN", "error", err, "arn", matchingProviderArn)
		return err
	}

	return nil
}
