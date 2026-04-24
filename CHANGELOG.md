# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Adopt pre-existing CloudFront Origin Access Identity by reading it from the matched distribution's S3 origin.
- Adopt pre-existing ACM certificate by matching `DomainName == irsa.<domain>` in `us-east-1`, preferring `ISSUED` on ties.
- New XRD status fields `status.importResources.cloudfrontOaiId` and `status.importResources.certificateArn`.

### Fixed

- Paginate `ListDistributions` when discovering the CloudFront distribution, so accounts with more than 100 distributions still match the IRSA one.

## [0.1.0] - 2026-04-22

### Changed

- Move Crossplane manifests (Function, XRD, Composition, RuntimeConfig) into Helm chart with configurable values.
- Add default tags to all resources
- Fetch all hosted zones in case of pagination
- Add sourceHash to detect drift serverside
- Sync workflows
- Use proper versioning when pushing the Crossplane function image

[Unreleased]: https://github.com/giantswarm/crossplane-fn-irsa/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/giantswarm/crossplane-fn-irsa/releases/tag/v0.1.0
