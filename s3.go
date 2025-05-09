package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"

	kclient "github.com/giantswarm/xfnlib/pkg/auth/kubernetes"
	"github.com/giantswarm/xfnlib/pkg/composite"

	"gopkg.in/square/go-jose.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type DiscoveryResponse struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	JwksURI                          string   `json:"jwks_uri"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
}

func IsChina(region string) bool {
	return strings.HasPrefix(region, "cn-")
}

func AWSEndpoint(region string) string {
	awsEndpoint := "amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		awsEndpoint = "amazonaws.com.cn"
	}
	return awsEndpoint
}

func (f *Function) GenerateDiscoveryFile(domain, bucketName, region string, patchTo string, composed *composite.Composition) error {
	// see https://github.com/aws/amazon-eks-pod-identity-webhook/blob/master/SELF_HOSTED_SETUP.md#create-the-oidc-discovery-and-keys-documents
	v := DiscoveryResponse{
		AuthorizationEndpoint:            "urn:kubernetes:programmatic_authorization",
		ResponseTypesSupported:           []string{"id_token"},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ClaimsSupported:                  []string{"sub", "iss"},
	}
	if !IsChina(region) {
		// Cloudfront
		v.Issuer = fmt.Sprintf("https://%s", domain)
		v.JwksURI = fmt.Sprintf("https://%s/keys.json", domain)
	} else {
		// Public S3 endpoint
		v.Issuer = fmt.Sprintf("https://s3.%s.%s/%s", region, AWSEndpoint(region), bucketName)
		v.JwksURI = fmt.Sprintf("https://s3.%s.%s/%s/keys.json", region, AWSEndpoint(region), bucketName)
	}

	b := &bytes.Buffer{}

	if err := json.NewEncoder(b).Encode(&v); err != nil {
		return fmt.Errorf("cannot encode to JSON: %w", err)
	}

	err := f.patchFieldValueToObject(patchTo, b.Bytes(), composed.DesiredComposite.Resource)
	return err
}

type KeyResponse struct {
	Keys []jose.JSONWebKey `json:"keys"`
}

func digestOfKey(key *rsa.PrivateKey) (string, error) {
	publicKeyDERBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to serialize public key to DER format: %v", err)
	}

	hasher := crypto.SHA256.New()
	hasher.Write(publicKeyDERBytes)
	publicKeyDERHash := hasher.Sum(nil)

	keyID := base64.RawURLEncoding.EncodeToString(publicKeyDERHash)

	return keyID, nil
}

func (f *Function) GenerateKeysFile(key *rsa.PrivateKey, patchTo string, composed *composite.Composition) error {
	var alg jose.SignatureAlgorithm

	kid, err := digestOfKey(key)
	if err != nil {
		return err
	}

	var keys []jose.JSONWebKey
	keys = append(keys, jose.JSONWebKey{
		Key:       key.Public(),
		KeyID:     kid,
		Algorithm: string(alg),
		Use:       "sig",
	})

	keyResponse := KeyResponse{Keys: keys}
	byt, err := json.MarshalIndent(keyResponse, "", "    ")
	if err != nil {
		return err
	}

	err = f.patchFieldValueToObject(patchTo, byt, composed.DesiredComposite.Resource)
	return err
}

func (f *Function) ServiceAccountSecret(clusterNamespace, clusterName string) (*rsa.PrivateKey, error) {
	oidcSecret := &v1.Secret{}
	client, err := kclient.Client()
	if err != nil {
		return nil, err
	}
	f.log.Debug("getting service account secret", "clusterNamespace", clusterNamespace, "clusterName", clusterName)
	err = client.Get(context.Background(), types.NamespacedName{Namespace: clusterNamespace, Name: clusterName + "-sa"}, oidcSecret)
	if err != nil {
		return nil, err
	}
	privBytes := oidcSecret.Data["tls.key"]
	block, _ := pem.Decode(privBytes)
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}
