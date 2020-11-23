# gh-go-rdeps

Find reverse Go dependencies across GitHub repositories. It supports [go modules](https://golang.org/ref/mod) and [dep](https://golang.github.io/dep/).

## Installation

```sh
go get github.com/pmatseykanets/gh-tools/cmd/gh-go-rdeps
```

## Usage

```txt
Usage: gh-go-rdeps [flags] <owner> <path>
  owner         Repository owner (user or organization)
  path          Module/package path

Flags:
  -help         Print this information and exit
  -progress     Show the progress
  -regexp=      Regexp to match repository names
  -version      Print the version and exit
```

## Environment variables

`GITHUB_TOKEN` shoud be set and contain GitHub personal access token

### Examples

Find all Go repositories that depend on `golang.org/x/sync`:

```sh
gh-go-rdeps owner golang.org/x/sync
```

Find all Go repositories that start with `api` and depend on `github.com/owner/library`

```sh
gh-go-rdeps -regexp '^api' owner github.com/owner/library
```
