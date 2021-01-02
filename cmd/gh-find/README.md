# gh-find

Walk file hierarchies across GitHub repositories.

## Installation

```sh
GO111MODULE=on go get github.com/pmatseykanets/gh-tools/cmd/gh-find
```

## Usage

```txt
Usage: gh-find [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository name

Flags:
  -help, h          Print this information and exit
  -branc            The branch name if different from the default
  -grep             The pattern to match the file contents. Implies
                      -type f
  -max-depth        Descend at most n directory levels
  -max-grep-results Limit the number of grep results
  -max-repo-results Limit the number of matched entries per repository
  -max-results      Limit the number of matched entries
  -min-depth        Descend at least n directory levels
  -name             The pattern to match the last component of the pathname
  -no-name          The pattern to reject the last component of the pathname
  -no-matches       List repositories with no matches. Implies
                      -max-results 0
                      -max-grep-results 1
                      -max-repo-results 1
  -no-path          The pattern to reject the pathname
  -path             The pattern to match the pathname
  -repo             The pattern to match repository names
  -size             Limit results based on the file size [+-]<d><u>
  -token            Prompt for an Access Token
  -type             The entry type f - file, d - directory
  -version          Print the version and exit
```

## Environment variables

`GHTOOLS_TOKEN` and `GITHUB_TOKEN` in the order of precedence can be used to set a GitHub access token.

### Examples

List all files in the default branch of the `golang/go` repository:

```sh
gh-find golang/go
```

List all files in the `release-branch.go1.15` branch of the `golang/go` repository:

```sh
gh-find -branch 'release-branch.go1.15' golang/go
```

List all files in all repositories in the `golang` GitHub organization:

```sh
gh-find golang
```

List all `README` and `LICENSE` files in all repositories in the `golang` GitHub organization but skip the ones in the `vendor` directories:

```sh
gh-find -name '^README$' -name '^LICENSE$' -no-path '^vendor/' -no-path '^src/vendor/' golang
```

List `README` files in the root directories of all repositories in the `golang` GitHub organization:

```sh
gh-find -name '^README$' -max-depth 1 golang
```

List all `LICENSE` files repositories which name starts with `go` in the `golang` GitHub organization:

```sh
gh-find -name '^LICENSE$' -repo '^go' golang
```

Find all `go.mod` files containing `golang.org/x/sync` in all repositories in the `golang` GitHub organization:

```sh
gh-find -name '^go.mod$' -grep 'golang.org/x/sync' golang
```
