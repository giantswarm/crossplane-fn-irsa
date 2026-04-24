package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
)

func TestExtractOaiIdFromDistribution(t *testing.T) {
	cases := map[string]struct {
		dist cloudfronttypes.DistributionSummary
		want string
	}{
		"nil origins": {
			dist: cloudfronttypes.DistributionSummary{},
			want: "",
		},
		"no items": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{},
			},
			want: "",
		},
		"origin without S3 config": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{{}},
				},
			},
			want: "",
		},
		"origin with nil OriginAccessIdentity": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{{
						S3OriginConfig: &cloudfronttypes.S3OriginConfig{},
					}},
				},
			},
			want: "",
		},
		"origin with empty OriginAccessIdentity": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{{
						S3OriginConfig: &cloudfronttypes.S3OriginConfig{
							OriginAccessIdentity: aws.String(""),
						},
					}},
				},
			},
			want: "",
		},
		"standard AWS path": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{{
						S3OriginConfig: &cloudfronttypes.S3OriginConfig{
							OriginAccessIdentity: aws.String("origin-access-identity/cloudfront/E1ABCDEF1234"),
						},
					}},
				},
			},
			want: "E1ABCDEF1234",
		},
		"trailing slash": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{{
						S3OriginConfig: &cloudfronttypes.S3OriginConfig{
							OriginAccessIdentity: aws.String("origin-access-identity/cloudfront/"),
						},
					}},
				},
			},
			want: "",
		},
		"no slashes": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{{
						S3OriginConfig: &cloudfronttypes.S3OriginConfig{
							OriginAccessIdentity: aws.String("E1ABCDEF1234"),
						},
					}},
				},
			},
			want: "",
		},
		"leading slash only": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{{
						S3OriginConfig: &cloudfronttypes.S3OriginConfig{
							OriginAccessIdentity: aws.String("/E1ABCDEF1234"),
						},
					}},
				},
			},
			want: "E1ABCDEF1234",
		},
		"skips origin without S3 config and returns next": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{
						{},
						{
							S3OriginConfig: &cloudfronttypes.S3OriginConfig{
								OriginAccessIdentity: aws.String("origin-access-identity/cloudfront/E2SECOND"),
							},
						},
					},
				},
			},
			want: "E2SECOND",
		},
		"returns first matching origin": {
			dist: cloudfronttypes.DistributionSummary{
				Origins: &cloudfronttypes.Origins{
					Items: []cloudfronttypes.Origin{
						{
							S3OriginConfig: &cloudfronttypes.S3OriginConfig{
								OriginAccessIdentity: aws.String("origin-access-identity/cloudfront/E1FIRST"),
							},
						},
						{
							S3OriginConfig: &cloudfronttypes.S3OriginConfig{
								OriginAccessIdentity: aws.String("origin-access-identity/cloudfront/E2SECOND"),
							},
						},
					},
				},
			},
			want: "E1FIRST",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := extractOaiIdFromDistribution(tc.dist)
			if got != tc.want {
				t.Errorf("extractOaiIdFromDistribution() = %q, want %q", got, tc.want)
			}
		})
	}
}
