# gh-purge-artifacts

Purge GitHub Actions artifacts.

## Installation

```sh
cd
GO111MODULE=on go get github.com/pmatseykanets/gh-tools/cmd/gh-purge-artifacts@latest
```

## Usage

```txt
Usage: gh-purge-artifacts [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository

Flags:
  -help         Print this information and exit
  -dry-run      Dry run
  -no-repo=     The pattern to reject repository names
  -repo         The pattern to match repository names
  -token        Prompt for an Access Token
  -version      Print the version and exit
```

## Environment variables

`GHTOOLS_TOKEN` and `GITHUB_TOKEN` in the order of precedence can be used to set a GitHub access token.

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
gh-purge-artifacts -repo '^api' owner
```

Dry-run mode. List found atifacts but don't purge.

```sh
gh-purge-artifacts -dry-run owner
```
