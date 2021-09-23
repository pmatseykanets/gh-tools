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
	usage := `Manage notification subscriptions across GitHub repositories

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
	owner        string
	repo         string
	repoRegexp   *regexp.Regexp
	token        bool           // Propmt for an access token.
	noRepoRegexp *regexp.Regexp // The pattern to reject repository names.
	watch        bool           // Subscribe to repository notifications.
	unwatch      bool           // Unsubscribe from repository notifications.
}

type subscriber struct {
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
		repo, noRepo          string
		err                   error
	)
	flag.BoolVar(&showHelp, "help", showHelp, "Print this information and exit")
	flag.StringVar(&noRepo, "no-repo", "", "The pattern to reject repository names")
	flag.StringVar(&repo, "repo", "", "The pattern to match repository names")
	flag.BoolVar(&config.token, "token", config.token, "Prompt for Access Token")
	flag.BoolVar(&config.unwatch, "unwatch", config.unwatch, "Unsubscribe from repository notifications")
	flag.BoolVar(&showVersion, "version", showVersion, "Print version and exit")
	flag.BoolVar(&config.watch, "watch", config.watch, "Subscribe to repository notifications")

	flag.Usage = usage
	flag.Parse()

	if showHelp {
		usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("gh-watch version %s\n", version.Version)
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

	if noRepo != "" {
		if config.noRepoRegexp, err = regexp.Compile(noRepo); err != nil {
			return config, fmt.Errorf("invalid no-repo pattern: %s", err)
		}
	}

	return config, nil
}

func run(ctx context.Context) error {
	var err error

	subscriber := &subscriber{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	subscriber.config, err = readConfig()
	if err != nil {
		return err
	}

	var token string
	if subscriber.config.token {
		token, _ = terminal.PasswordPrompt("Access Token: ")
	} else {
		token = auth.GetToken()
	}
	if token == "" {
		return fmt.Errorf("access token is required")
	}

	subscriber.gh = github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)))

	return subscriber.run(ctx)
}

func subscriptionStatus(sub *github.Subscription) string {
	if sub == nil {
		return "not watching"
	}

	if sub.GetIgnored() {
		return "ignoring"
	}

	return "watching"
}

func (w *subscriber) run(ctx context.Context) error {
	repos, err := gh.NewRepoFinder(w.gh).Find(ctx, gh.RepoFilter{
		Owner:      w.config.owner,
		Repo:       w.config.repo,
		RepoRegexp: w.config.repoRegexp,
	})
	if err != nil {
		return err
	}

	for _, repo := range repos {
		fmt.Fprint(w.stdout, repo.GetFullName())

		// Get the current subscription for the repo.
		sub, _, err := w.gh.Activity.GetRepositorySubscription(ctx, w.config.owner, repo.GetName())
		if err != nil {
			fmt.Fprintln(w.stdout)
			return err
		}

		// List the current subscription status.
		fmt.Fprint(w.stdout, " ", subscriptionStatus(sub))

		switch {
		case w.config.watch && !sub.GetSubscribed():
			sub, _, err = w.gh.Activity.SetRepositorySubscription(ctx, w.config.owner, repo.GetName(), &github.Subscription{
				Subscribed: github.Bool(true),
			})
			if err != nil {
				fmt.Fprintln(w.stdout)
				return err
			}

			fmt.Fprint(w.stdout, " -> ", subscriptionStatus(sub))
		case w.config.unwatch && sub.GetSubscribed():
			_, err = w.gh.Activity.DeleteRepositorySubscription(ctx, w.config.owner, repo.GetName())
			if err != nil {
				fmt.Fprintln(w.stdout)
				return err
			}
			sub = nil

			fmt.Fprint(w.stdout, " -> ", subscriptionStatus(sub))
		}

		fmt.Fprintln(w.stdout)
	}

	return nil
}
