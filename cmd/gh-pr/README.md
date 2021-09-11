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
  -no-repo=         The pattern to reject repository names
  -repo=            The pattern to match repository names
  -review=          The GitHub user login to request the PR review from
  -script=          The script to apply changes
  -script-file=     Read the script from a file
  -shell=           The shell to use to run the script. Default bash
  -title=           The PR title
  -token            Prompt for an Access Token
  -version          Print the version and exit
```

## Environment variables

`GHTOOLS_TOKEN` and `GITHUB_TOKEN` in the order of precedence can be used to set a GitHub access token.

### Example

Walk over all repositories with names starting with `api-` in the GitHub organization `org` and for each repository upgrade `aws-sdk-go` to `v1.35.0`, create a respective pull request, assign it to `john` and request PR reviews from `chris` and `linda`

```sh
gh-pr -branch upgrade-aws-sdk-to-1-35 \
-assign john -review chris -review linda \
-title 'Update aws-sdk-go to v1.35.0' \
-desc 'Ref: issue#123' \
-script-file "$HOME/src/scripts/upgrade-aws-sdk.sh" \
-repo '^api-' org
```

where `$HOME/src/scripts/upgrade-aws-sdk.sh` may look like this

```sh
#!/usr/bin/env bash

if [ ! -f go.mod ]; then
    echo "There is no go.mod"
    exit
fi

version=$(go list -m -f '{{ .Version }}' github.com/aws/aws-sdk-go)
if [ -z "$version" ]; then
    echo "No aws-sdk-go dependency"
    exit
fi
if [ "$version" == "v1.35.0" ]; then
    echo "Already using aws-sdk-go v1.35.0"
    exit
fi

go mod edit -require='github.com/aws/aws-sdk-go@v1.35.0'
go mod tidy
```
