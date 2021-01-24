# gh-tools

![gh-find image](https://user-images.githubusercontent.com/779965/105617575-09331480-5dad-11eb-82cb-bde13473aa4f.png)

GitHub productivity tools

- [gh-purge-artifacts](cmd/gh-purge-artifacts) Purge GitHub Actions artifacts across GitHub repositories
- [gh-go-rdeps](cmd/gh-go-rdeps) Find reverse Go dependencies across GitHub repositories
- [gh-find](cmd/gh-find) Walk file hierarchies across GitHub repositories
- [gh-pr](cmd/gh-pr) Automate PR creation across GitHub repositories

## Authentication

All tools require a GitHub personal access token in order to authenticate API requests and use following methods, in the order of precedence, to infer/set the token:

- `-token` flag, in which case the user will be asked to enter the token interactively
- `GHTOOLS_TOKEN` environment variable
- `GITHUB_TOKEN` environment variable
- `~/.config/gh-tools/auth.yml` file, containing the token

    ```yaml
    oauth_token: <token>
    ```

- GitHub's official CLI tool [`gh`](https://github.com/cli/cli) configuration file, to avoid creating separate personal accesss tokens

Here's how you can [create a personal access token](https://docs.github.com/en/github/authenticating-to-github/creating-a-personal-access-token).

Your personal access token may need the following scopes to use `gh-tools`:

- `repo`
- `workflow`
- `read:user`

The explicit `worklow` scope is requred if you want to be able to make changes to GitHub Actions workflow files with `gh-pr` tool.
