package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pmatseykanets/gh-tools/auth"
	gh "github.com/pmatseykanets/gh-tools/github"
	"github.com/pmatseykanets/gh-tools/terminal"
	"github.com/pmatseykanets/gh-tools/version"
	"golang.org/x/oauth2"
)

func usage() {
	usage := `Purge GitHub Actions Artifacts across GitHub repositories

Usage: gh-purge-artifacts [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository name

Flags:
  -help         Print this information and exit
  -dry-run      Dry run
  -repo         The pattern to match repository names
  -token        Prompt for an Access Token
  -version      Print the version and exit
`
	fmt.Println(usage)
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Printf("error: %s\n", err)
		os.Exit(1)
	}
}

type config struct {
	owner      string
	repo       string
	repoRegexp *regexp.Regexp
	dryRun     bool
	token      bool // Propmt for an access token.
}

type purger struct {
	gh     *github.Client
	config config
	stdout io.WriteCloser
	stderr io.WriteCloser
}

func readConfig() (config, error) {
	if len(os.Args) == 0 {
		usage()
		os.Exit(1)
	}

	config := config{}

	var (
		showVersion, showHelp bool
		repo                  string
		err                   error
	)
	flag.BoolVar(&config.dryRun, "dry-run", config.dryRun, "Dry run")
	flag.BoolVar(&showHelp, "help", showHelp, "Print this information and exit")
	flag.StringVar(&repo, "repo", "", "The pattern to match repository names")
	flag.BoolVar(&config.token, "token", config.token, "Prompt for Access Token")
	flag.BoolVar(&showVersion, "version", showVersion, "Print version and exit")
	flag.Usage = usage
	flag.Parse()

	if showHelp {
		usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("gh-purge-artifacts version %s\n", version.Version)
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

	if repo != "" {
		config.repoRegexp, err = regexp.Compile(repo)
		if err != nil {
			return config, fmt.Errorf("invalid name pattern: %s: %s", repo, err)
		}
	}

	return config, nil
}

func run(ctx context.Context) error {
	var err error

	purger := &purger{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	purger.config, err = readConfig()
	if err != nil {
		return err
	}

	var token string
	if purger.config.token {
		token, _ = terminal.PasswordPrompt("Access Token: ")
	} else {
		token = auth.GetToken()
	}
	if token == "" {
		return fmt.Errorf("access token is required")
	}

	purger.gh = github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)))

	return purger.purge(ctx)
}

func (p *purger) purge(ctx context.Context) error {
	repoFinder := gh.RepoFinder{
		Client:     p.gh,
		Owner:      p.config.owner,
		Repo:       p.config.repo,
		RepoRegexp: p.config.repoRegexp,
	}
	repos, err := repoFinder.Find(ctx)
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
		fmt.Fprintf(p.stdout, "Total:")
		if p.config.dryRun {
			fmt.Fprintf(p.stdout, " found")
		} else {
			fmt.Fprintf(p.stdout, " purged")
		}
		fmt.Fprintf(p.stdout, " %d artifacts (%s) in %d repos\n", totalDeleted, formatSize(totalSize), totalRepos)
	}

	return nil
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

	fmt.Fprintf(p.stdout, "%s/%s", owner, name)

	var deleted, size int64
	defer func() {
		if deleted > 0 {
			if p.config.dryRun {
				fmt.Fprintf(p.stdout, " found")
			} else {
				fmt.Fprintf(p.stdout, " purged")
			}
			fmt.Fprintf(p.stdout, " %d out of %d artifacts (%s)", len(artifacts), deleted, formatSize(size))
		}
		fmt.Fprintln(p.stdout)
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
