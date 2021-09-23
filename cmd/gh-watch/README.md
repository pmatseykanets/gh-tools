# gh-watch

Manage notification subscriptions across GitHub repositories.

## Installation

```sh
cd
GO111MODULE=on go get github.com/pmatseykanets/gh-tools/cmd/gh-watch@latest
```

## Usage

```txt
Usage: gh-watch [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository name

Flags:
  -help         Print this information and exit
  -no-repo=     The pattern to reject repository names
  -repo=        The pattern to match repository names
  -token        Prompt for an Access Token
  -unwatch      Unsubscribe from repository notifications
  -version      Print the version and exit
  -watch        Subscribe to repository notifications
```

## Environment variables

`GHTOOLS_TOKEN` and `GITHUB_TOKEN` in the order of precedence can be used to set a GitHub access token.

### Examples

List the subscription status for all repositories in the GitHub org `foo`:

```sh
gh-watch foo
```

Unsubscribe from all repositories in the GitHub org foo:

```sh
gh-watch -unwatch foo
```

Subscribe to notifications for repositories starting with `api-` in the GitHub org foo:

```sh
gh-watch -watch -repo '$api-' foo
```
