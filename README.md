# Woodpecker CI Config Service

<p align="center">
  <a href="https://ci.opencloud.eu/repos/5" title="Pipeline Status">
    <img src="https://ci.opencloud.eu/api/badges/5/status.svg" alt="Pipeline Status">
  </a>
  <a href="https://matrix.to/#/#woodpecker:matrix.org" title="Join the Matrix space at https://matrix.to/#/#opencloud:matrix.org">
    <img src="https://img.shields.io/matrix/opencloud:matrix.org?label=matrix" alt="Matrix space">
  </a>
  <a href="https://goreportcard.com/report/github.com/opencloud-eu/woodpecker-ci-config-service" title="Go Report Card">
    <img src="https://goreportcard.com/badge/github.com/opencloud-eu/woodpecker-ci-config-service" alt="Go Report Card">
  </a>
  <a href="https://pkg.go.dev/github.com/opencloud-eu/woodpecker-ci-config-service" title="go reference">
    <img src="https://pkg.go.dev/badge/github.com/opencloud-eu/woodpecker-ci-config-service" alt="go reference">
  </a>
  <a href="https://github.com/opencloud-eu/woodpecker-ci-config-service/releases/latest" title="GitHub release">
    <img src="https://img.shields.io/github/v/release/opencloud-eu/woodpecker-ci-config-service?sort=semver" alt="GitHub release">
  </a>
  <a href="https://opensource.org/licenses/Apache-2.0" title="License: Apache-2.0">
    <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License: Apache-2.0">
  </a>
</p>

`woodpecker-ci-config-service` is a command-line interface (CLI) tool designed to work with Woodpecker CI configurations. It provides functionalities to convert and serve configuration files for Woodpecker CI, a powerful and flexible continuous integration system.

<p align="center">
  <img src="assets/logo.png">
</p>

## Features

- **Convert Command** – Convert configuration files from a source format to Woodpecker CI format.
- **Server Command** – Serve configuration files through a web service for CI runs.

## Why Use Woodpecker CI Config Service?

When integrating Woodpecker CI into your development workflow, `woodpecker-ci-config-service`
helps streamline the process of managing and serving configuration files.
It provides:

- Easy conversion from supported formats (currently Starlark) to Woodpecker CI configuration.
- Flexible output options, either to stdout or to disk.
- A web server to serve configurations to CI runs directly.

## Commands

### Convert Command

The `convert` command converts configuration files from a source format to Woodpecker CI format.
Currently, it supports conversion from Starlark.

#### Usage

```sh
# using the forge environment
ENV_SECRET_GITHUB_TOKEN=XXX wccs convert testdata/convert.forge.json [--out <output-file>]
# using the fs environment
WCCS_CONVERT_PROVIDERS=fs WCCS_CONVERT_PROVIDER_FS_SOURCE=testdata/*.star wccs convert testdata/convert.fs.json [--out <output-file>]
```

### Server Command

The `server` command starts a web server that serves configuration files for CI runs.

#### Usage

```sh
wccs server
```

## Installation

To install `woodpecker-ci-config-service`, clone the repository and build the tool:

```sh
git clone https://github.com/opencloud-eu/woodpecker-ci-config-service.git
cd woodpecker-ci-config-service
go build -o bin/wccs cmd/wccs/*.go
```

## Future Improvements

- Support for additional source formats for conversion.
- Support for additional forges and private repositories.
- Improved error handling and logging.

## License

This project is licensed under the [Apache License 2.0](LICENSE).

## Contributions

Contributions are welcome! Feel free to submit issues or pull requests to improve `woodpecker-ci-config-service`.

## Acknowledgements

Special thanks to the Woodpecker CI team for their amazing work on the [Woodpecker CI](https://github.com/woodpecker-ci/woodpecker) project.

The logo was generated using DALL-E. Thanks to the DALL-E team for providing such a fantastic tool.

## Contact

For more information, reach out via the [GitHub Issues](https://github.com/opencloud-eu/woodpecker-ci-config-service/issues).
