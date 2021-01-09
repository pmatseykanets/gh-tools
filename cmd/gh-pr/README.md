# gh-pr

Automate PR creation across GitHub repositories.

## Installation

```sh
cd
GO111MODULE=on go get github.com/pmatseykanets/gh-tools/cmd/gh-pr@latest
```

## Usage

```txt
Usage: gh-pr [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository name

Flags:
  -assign=          The GitHub user login to assign the PR to
  -help, h          Print this information and exit
  -branch=          The branch name if different from the default
  -desc=            The PR description
  -no-fork          Don't include fork repositories
  -no-private       Don't include private repositories
  -no-public        Don't include public repositories
  -repo=            The pattern to match repository names
  -review=          The GitHub user login to request the PR review from
  -script=          The script to apply changes
  -shell=           The shell to use to run the script. Default bash
  -title=           The PR title
  -token            Prompt for an Access Token
  -version          Print the version and exit
```

## Environment variables

`GHTOOLS_TOKEN` and `GITHUB_TOKEN` in the order of precedence can be used to set a GitHub access token.

### Examples

TBD.
