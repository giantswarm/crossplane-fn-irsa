# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add GitHub Actions workflow to bump `api/composition/function.yaml` package version on release PRs.
- Move Crossplane manifests (Function, XRD, Composition, RuntimeConfig) into Helm chart with configurable values.

### Changed

- Add default tags to all resources
- Fetch all hosted zones in case of pagination
- Add sourceHash to detect drift serverside
- Sync workflows
- Use proper versioning when pushing the Crossplane function image

[Unreleased]: https://github.com/giantswarm/crossplane-fn-irsa/tree/main
