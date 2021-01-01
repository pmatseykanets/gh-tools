# gh-tools

GitHub Tools

- [gh-purge-artifacts](cmd/gh-purge-artifacts) Purge GitHub Actions artifacts across GitHub repositories
- [gh-go-rdeps](cmd/gh-go-rdeps) Find reverse Go dependencies across GitHub repositories
- [gh-find](cmd/gh-find) Walk file hierarchies across GitHub repositories

## Authentication

All tools require a GitHub access token in order to authenticate API requests and use following methods, in the order of precedence, to infer/set the token:

- If `-token` flag is used a user will be asked to enter the token interactively
- `GHTOOLS_TOKEN` environment variable
- `GITHUB_TOKEN` environment variable
- `~/.config/gh-tools/auth.yml` file, containing the token

    ```yaml
    oauth_token: <token>
    ```

- `gh cli`'s configuration file
