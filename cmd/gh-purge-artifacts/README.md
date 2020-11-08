# gh-purge-artifacts

Purge GitHub Actions artifacts.

## Installation

```sh
go get github.com/pmatseykanets/gh/cmd/gh-purge-artifacts
```

## Usage

```txt
Usage: gh-purge-artifacts [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository

Flags:
  -help         Print this information and exit
  -dry-run      Dry run
  -regexp=      Regexp to match repository names
  -version      Print the version and exit
```

## Environment variables

`GITHUB_TOKEN` shoud be set and contain GitHub personal access token

### Examples

Single repository mode:

```sh
gh-purge-artifacts owner/repo
```

Purge artifacts in all repositories

```sh
gh-purge-artifacts owner
```

Purge artifacts in repositories starting with 'api'

```sh
gh-purge-artifacts -regexp '^api' owner
```

Dry-run mode. List found atifacts but don't purge.

```sh
gh-purge-artifacts -dry-run owner
```
