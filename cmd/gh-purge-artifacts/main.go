package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

const version = "0.1.0"

func usage() {
	usage := `A tool to purge GitHub Actions Artifacts 

Usage: gh-purge-artifacts [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository name

Flags:
  -help         Print this information and exit
  -dry-run      Dry run
  -regexp=      Regexp to match repository names
  -version      Print the version and exit

Environment variables:
  GITHUB_TOKEN  an authentication token for github.com API requests

Examples:
  gh-purge-artifacts john/website
  gh-purge-artifacts -regexp '^api' john
  gh-purge-artifacts -dry-run acme
`
	fmt.Printf("gh-purge-artifacts %s\n", version)
	fmt.Println(usage)
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Printf("error: %s\n", err)
		os.Exit(1)
	}
}

type config struct {
	owner  string
	repo   string
	regexp *regexp.Regexp
	dryRun bool
}

type purger struct {
	gh     *github.Client
	config config
}

func readConfig() (config, error) {
	if len(os.Args) == 0 {
		usage()
		os.Exit(1)
	}

	config := config{}

	var showVersion, showHelp bool
	var nameRegExp string
	var err error

	flag.BoolVar(&config.dryRun, "dry-run", config.dryRun, "Dry run")
	flag.BoolVar(&showHelp, "help", showHelp, "Print this information and exit")
	flag.StringVar(&nameRegExp, "regexp", "", "Regexp to match repository names")
	flag.BoolVar(&showVersion, "version", showVersion, "Print version and exit")
	flag.Usage = usage
	flag.Parse()

	if showHelp {
		usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("gh-purge-artifacts version %s\n", version)
		os.Exit(0)
	}

	parts := strings.Split(flag.Arg(0), "/")
	nparts := len(parts)
	if nparts > 0 {
		config.owner = parts[0]
	}
	if nparts > 1 {
		config.repo = parts[1]
	}
	if nparts > 2 {
		return config, fmt.Errorf("invalid owner or repository name %s", flag.Arg(0))
	}

	if config.owner == "" {
		return config, fmt.Errorf("owner is required")
	}

	if nameRegExp != "" {
		config.regexp, err = regexp.Compile(nameRegExp)
		if err != nil {
			return config, fmt.Errorf("invalid name pattern: %s", err)
		}
	}

	return config, nil
}

func run(ctx context.Context) error {
	var err error
	purger := &purger{}
	purger.config, err = readConfig()
	if err != nil {
		return err
	}

	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		return fmt.Errorf("GITHUB_TOKEN env variable should be set")
	}

	purger.gh = github.NewClient(
		oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: ghToken}),
		),
	)

	return purger.purge(ctx)
}

func (p *purger) purge(ctx context.Context) error {
	repos, err := p.getRepos(ctx)
	if err != nil {
		return err
	}

	var totalDeleted, totalSize int64
	for _, repo := range repos {
		deleted, size, err := p.purgeRepoArtifacts(ctx, repo)
		if err != nil {
			return err
		}
		totalDeleted += deleted
		totalSize += size
	}

	if totalRepos := len(repos); totalRepos > 1 {
		fmt.Printf("Total:")
		if p.config.dryRun {
			fmt.Printf(" found")
		} else {
			fmt.Printf(" purged")
		}
		fmt.Printf(" %d artifacts (%s) in %d repos\n", totalDeleted, formatSize(totalSize), totalRepos)
	}

	return nil
}

func (p *purger) getSingleRepo(ctx context.Context) (*github.Repository, error) {
	repo, _, err := p.gh.Repositories.Get(ctx, p.config.owner, p.config.repo)
	if err != nil {
		return nil, fmt.Errorf("can't read repository: %s", err)
	}

	return repo, nil
}

func (p *purger) getUserRepos(ctx context.Context) ([]*github.Repository, error) {
	opt := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: 30},
		Affiliation: "owner",
	}
	var list []*github.Repository
	for {
		repos, resp, err := p.gh.Repositories.List(ctx, p.config.owner, opt)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		if p.config.regexp == nil {
			list = append(list, repos...)
		} else {
			for _, repo := range repos {
				if p.config.regexp.Match([]byte(repo.GetName())) {
					list = append(list, repo)
				}
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return list, nil
}

func (p *purger) getOrgRepos(ctx context.Context) ([]*github.Repository, error) {
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 30},
	}
	var list []*github.Repository
	for {
		repos, resp, err := p.gh.Repositories.ListByOrg(ctx, p.config.owner, opt)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		if p.config.regexp == nil {
			list = append(list, repos...)
		} else {
			for _, repo := range repos {
				if p.config.regexp.Match([]byte(repo.GetName())) {
					list = append(list, repo)
				}
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return list, nil
}

func (p *purger) getRepos(ctx context.Context) ([]*github.Repository, error) {
	owner, _, err := p.gh.Users.Get(ctx, p.config.owner)
	if err != nil {
		return nil, fmt.Errorf("can't read owner information: %s", err)
	}

	// A single repository.
	if p.config.repo != "" {
		repo, err := p.getSingleRepo(ctx)
		if err != nil {
			return nil, err
		}
		return []*github.Repository{repo}, nil
	}

	var repos []*github.Repository
	switch t := owner.GetType(); t {
	case "User":
		repos, err = p.getUserRepos(ctx)
	case "Organization":
		repos, err = p.getOrgRepos(ctx)
	default:
		err = fmt.Errorf("unknown owner type %s", t)
	}

	return repos, err
}

func (p *purger) purgeRepoArtifacts(ctx context.Context, repo *github.Repository) (int64, int64, error) {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()

	var artifacts []*github.Artifact
	opt := &github.ListOptions{PerPage: 30}
	for {
		list, resp, err := p.gh.Actions.ListArtifacts(ctx, owner, name, opt)
		if err != nil {
			return 0, 0, err
		}

		artifacts = append(artifacts, list.Artifacts...)

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	fmt.Printf("%s/%s", owner, name)

	var deleted, size int64
	defer func() {
		if deleted > 0 {
			if p.config.dryRun {
				fmt.Printf(" found")
			} else {
				fmt.Printf(" purged")
			}
			fmt.Printf(" %d out of %d artifacts (%s)", len(artifacts), deleted, formatSize(size))
		}
		fmt.Println()
	}()
	for _, artifact := range artifacts {
		if !p.config.dryRun {
			_, err := p.gh.Actions.DeleteArtifact(ctx, owner, name, artifact.GetID())
			if err != nil {
				return 0, 0, err
			}
		}

		deleted++
		size += artifact.GetSizeInBytes()
	}

	return deleted, size, nil
}

func formatSize(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n := n / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
