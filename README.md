## Modifications in the fork
- [Make the package importable (remove main fn, rename the pkg)](https://github.com/keboola/go-oauth2-proxy/commit/789f4d1f85c8d2257b614c154407b1058d9d0c8a)
- [Added ability to add a custom middleware](https://github.com/keboola/go-oauth2-proxy/commit/bf4d486c80855a85ee956aee14c6cf3ef4d85407)
- [Added middlewares chain for upstream handler](https://github.com/keboola/go-oauth2-proxy/commit/c74325648bd7364d829cd9464fe2b6488bfbb260)
- [Moved some locks to prevent race detections](https://github.com/keboola/go-oauth2-proxy/commit/d4f9ab6b7147eb9d34be4441ce50db2d61c4d0f2)
- [Added workaround for CSRF cookie](https://github.com/keboola/go-oauth2-proxy/commit/5486c9884b2953ba1a2d4efd0c89f527953104d1)
- [Added option to configure upstream transport](https://github.com/keboola/go-oauth2-proxy/commit/8bde35a277a8548e12e9ad01f0658da22cbf04b6)
- [Added argument for custom error handler](https://github.com/keboola/go-oauth2-proxy/commit/f9b5f906a4f40da25437425cc4e4b5a932ca2f36) 
- [Added option `UpstreamHandler`](https://github.com/keboola/go-oauth2-proxy/commit/2b0c2a99d50ba80b68ffba8175ebc3b9311f6496)
- [Added option `OnNeedsLogin`](https://github.com/keboola/go-oauth2-proxy/commit/efea780d43062f8a61c84cda65e6742835f64dbd)
- [Added request argument to `WriteErrorPage`](https://github.com/keboola/go-oauth2-proxy/commit/757d1371618332e79bc70a6371654b740b754a26)


## Fork maintenance

In order to update this fork to a new version of the base repository follow these steps:

- Create a new branch from our `master` branch
- Merge latest tag from upstream and fix conflicts
- Send a PR to this repository (not upstream)
- Fix potential pipeline issues and ask for review

After merging, change the required version in `go.mod` to `master` and run `go mod tidy` and `go mod vendor`. Double check if it updated to the intended commit. Try [disabling Go Proxy](https://webtips.fly.dev/how-to-disable-go-proxy/) if the commit is [incorrect](https://proxy.golang.org/#faq-new-version).

------

![OAuth2 Proxy](docs/static/img/logos/OAuth2_Proxy_horizontal.svg)

[![Continuous Integration](https://github.com/oauth2-proxy/oauth2-proxy/actions/workflows/ci.yaml/badge.svg)](https://github.com/oauth2-proxy/oauth2-proxy/actions/workflows/ci.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/oauth2-proxy/oauth2-proxy)](https://goreportcard.com/report/github.com/oauth2-proxy/oauth2-proxy)
[![GoDoc](https://godoc.org/github.com/oauth2-proxy/oauth2-proxy?status.svg)](https://godoc.org/github.com/oauth2-proxy/oauth2-proxy)
[![MIT licensed](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![Maintainability](https://api.codeclimate.com/v1/badges/a58ff79407212e2beacb/maintainability)](https://codeclimate.com/github/oauth2-proxy/oauth2-proxy/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/a58ff79407212e2beacb/test_coverage)](https://codeclimate.com/github/oauth2-proxy/oauth2-proxy/test_coverage)

A reverse proxy and static file server that provides authentication using Providers (Google, Keycloak, GitHub and others)
to validate accounts by email, domain or group.

**Note:** This repository was forked from [bitly/OAuth2_Proxy](https://github.com/bitly/oauth2_proxy) on 27/11/2018.
Versions v3.0.0 and up are from this fork and will have diverged from any changes in the original fork.
A list of changes can be seen in the [CHANGELOG](CHANGELOG.md).

**Note:** This project was formerly hosted as `pusher/oauth2_proxy` but has been renamed as of 29/03/2020 to `oauth2-proxy/oauth2-proxy`.
Going forward, all images shall be available at `quay.io/oauth2-proxy/oauth2-proxy` and binaries will be named `oauth2-proxy`.

![Sign In Page](docs/static/img/sign-in-page.png)

## Installation

1.  Choose how to deploy:

    a. Using a [Prebuilt Binary](https://github.com/oauth2-proxy/oauth2-proxy/releases) (current release is `v7.6.0`)

    b. Using Go to install the latest release
    ```bash
    $ go install github.com/oauth2-proxy/oauth2-proxy/v7@latest
    # which will put the binary in `$GOROOT/bin`
    ```
    c. Using a [Prebuilt Docker Image](https://quay.io/oauth2-proxy/oauth2-proxy) (AMD64, PPC64LE, ARMv6, ARMv7, and ARM64 available)

    d. Using a [Pre-Release Nightly Docker Image](https://quay.io/oauth2-proxy/oauth2-proxy-nightly) (AMD64, PPC64LE, ARMv6, ARMv7, and ARM64 available)

    e. Using the official [Kubernetes manifest](https://github.com/oauth2-proxy/manifests) (Helm)

    Prebuilt binaries can be validated by extracting the file and verifying it against the `sha256sum.txt` checksum file provided for each release starting with version `v3.0.0`.

    ```
    sha256sum -c sha256sum.txt 2>&1 | grep OK
    oauth2-proxy-x.y.z.linux-amd64: OK
    ```

2.  [Select a Provider and Register an OAuth Application with a Provider](https://oauth2-proxy.github.io/oauth2-proxy/docs/configuration/oauth_provider)
3.  [Configure OAuth2 Proxy using config file, command line options, or environment variables](https://oauth2-proxy.github.io/oauth2-proxy/docs/configuration/overview)
4.  [Configure SSL or Deploy behind a SSL endpoint](https://oauth2-proxy.github.io/oauth2-proxy/docs/configuration/tls) (example provided for Nginx)


## Security

If you are running a version older than v6.0.0 we **strongly recommend you please update** to a current version.
See [open redirect vulnerability](https://github.com/oauth2-proxy/oauth2-proxy/security/advisories/GHSA-5m6c-jp6f-2vcv) for details.

## Docs

Read the docs on our [Docs site](https://oauth2-proxy.github.io/oauth2-proxy/docs/).

![OAuth2 Proxy Architecture](docs/static/img/architecture.svg)

## Images

From `v7.6.0` and up the base image has been changed from Alpine to [GoogleContainerTools/distroless](https://github.com/GoogleContainerTools/distroless).
This image comes with even fewer installed dependencies and thus should improve security. The image therefore is also slightly smaller than Alpine.
For debugging purposes (and those who really need it (i.e. `armv6`)) we still provide images based on Alpine. The tags of these images are suffixed with `-alpine`.

Since 2023-11-18 we provide nightly images. These images are build and pushed nightly to `quay.io/oauth2-proxy/oauth2-proxy-nightly` from `master`.
These images should be considered alpha and therefore *should not* be used for production purposes unless you know what you're doing.

## Getting Involved

If you would like to reach out to the maintainers, come talk to us in the `#oauth2-proxy` channel in the [Gophers slack](http://gophers.slack.com/).

## Contributing

Please see our [Contributing](CONTRIBUTING.md) guidelines. For releasing see our [release creation guide](RELEASE.md).
