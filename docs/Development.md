# Development

## Prerequisites

- GoLang version greater than or equal to version in **go.mod**

## Setup

```bash
# install golanglint-ci for linting
https://golangci-lint.run/usage/install/#local-installation

# install vulnerability checker
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## Building

```bash
make
```