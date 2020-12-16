package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pmatseykanets/gh-tools/version"
	"golang.org/x/mod/modfile"
	"golang.org/x/oauth2"
)

func usage() {
	usage := `Find reverse Go dependencies across GitHub repositories

Usage: gh-go-rdeps [flags] <owner> <path>
  owner         Repository owner (user or organization)
  path          Module/package path

Flags:
  -help         Print this information and exit
  -progress     Show the progress
  -regexp=      Regexp to match repository names
  -version      Print the version and exit

Environment variables:
  GITHUB_TOKEN  an authentication token for github.com API requests
`
	fmt.Println(usage)
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

type config struct {
	owner    string
	modpath  string
	regexp   *regexp.Regexp
	progress bool
}

type finder struct {
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
		nameRegExp            string
		err                   error
	)

	flag.BoolVar(&showHelp, "help", showHelp, "Print this information and exit")
	flag.BoolVar(&config.progress, "progress", config.progress, "Show progress")
	flag.StringVar(&nameRegExp, "regexp", "", "Regexp to match repository names")
	flag.BoolVar(&showVersion, "version", showVersion, "Print version and exit")
	flag.Usage = usage
	flag.Parse()

	if showHelp {
		usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("gh-go-rdeps version %s\n", version.Version)
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		return config, fmt.Errorf("owner is required")
	}
	config.owner = strings.TrimSpace(flag.Arg(0))
	if config.owner == "" {
		return config, fmt.Errorf("owner can't be empty")
	}

	if flag.NArg() < 2 {
		return config, fmt.Errorf("mod path is required")
	}
	config.modpath = strings.TrimSpace(flag.Arg(1))
	if config.modpath == "" {
		return config, fmt.Errorf("mod path can't be empty")
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

	finder := &finder{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	finder.config, err = readConfig()
	if err != nil {
		return err
	}

	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		return fmt.Errorf("GITHUB_TOKEN env variable should be set")
	}

	finder.gh = github.NewClient(
		oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: ghToken}),
		),
	)

	return finder.find(ctx)
}

func (f *finder) find(ctx context.Context) error {
	repos, err := f.getRepos(ctx)
	if err != nil {
		return err
	}

	var (
		repo         *github.Repository
		goRepo       bool
		contents     []byte
		mod          *modfile.File
		require      *modfile.Require
		replace      *modfile.Replace
		gopkg        *Gopkg
		gopkgProject GopkgProject
		dependencies []string
	)
repos:
	for _, repo = range repos {
		if f.config.progress {
			fmt.Fprint(f.stderr, ".")
		}

		goRepo, err = f.goRepo(ctx, repo)
		if err != nil {
			return err
		}

		if !goRepo {
			continue
		}

		// go modules take precedence.
		contents, err = f.getFileContents(ctx, repo, "go.mod")
		if err != nil {
			return err
		}

		if len(contents) > 0 {
			mod, err = modfile.Parse("go.mod", contents, nil)
			if err != nil {
				return err
			}

			for _, require = range mod.Require {
				if strings.HasPrefix(require.Mod.Path, f.config.modpath) {
					dependencies = append(dependencies, mod.Module.Mod.Path)
					continue repos
				}
			}
			for _, replace = range mod.Replace {
				if strings.HasPrefix(replace.Old.Path, f.config.modpath) ||
					strings.HasPrefix(replace.New.Path, f.config.modpath) {
					dependencies = append(dependencies, mod.Module.Mod.Path)
					continue repos
				}
			}
			continue repos
		}

		// Gopkg.toml.
		contents, err = f.getFileContents(ctx, repo, "Gopkg.toml")
		if err != nil {
			return err
		}

		if len(contents) == 0 {
			continue repos
		}

		gopkg, err = parseGopkg(bytes.NewReader(contents))
		if err != nil {
			return err
		}

		for _, gopkgProject = range gopkg.Constraints {
			if strings.HasPrefix(gopkgProject.Name, f.config.modpath) ||
				strings.HasPrefix(gopkgProject.Source, f.config.modpath) {
				dependencies = append(dependencies, fmt.Sprintf("github.com/%s/%s", f.config.owner, repo.GetName()))
				continue repos
			}
		}
		for _, gopkgProject = range gopkg.Overrides {
			if strings.HasPrefix(gopkgProject.Name, f.config.modpath) ||
				strings.HasPrefix(gopkgProject.Source, f.config.modpath) {
				dependencies = append(dependencies, fmt.Sprintf("github.com/%s/%s", f.config.owner, repo.GetName()))
				continue repos
			}
		}
	}

	sort.Strings(dependencies)

	if f.config.progress {
		fmt.Fprintln(f.stderr)
	}
	for _, dependency := range dependencies {
		fmt.Fprintln(f.stdout, dependency)
	}

	return nil
}

func (f *finder) getFileContents(ctx context.Context, repo *github.Repository, filename string) ([]byte, error) {
	fileContents, _, resp, err := f.gh.Repositories.GetContents(ctx, f.config.owner, repo.GetName(), filename, nil)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}

	contents, err := fileContents.GetContent()
	if err != nil {
		return nil, err
	}

	return []byte(contents), nil
}

func (f *finder) goRepo(ctx context.Context, repo *github.Repository) (bool, error) {
	tree, resp, err := f.gh.Git.GetTree(ctx, f.config.owner, *repo.Name, "master", true)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusConflict {
			return false, nil
		}
		return false, err
	}

	for _, entry := range tree.Entries {
		if strings.HasSuffix(entry.GetPath(), ".go") ||
			strings.HasSuffix(entry.GetPath(), "Gopkg.toml") ||
			strings.HasSuffix(entry.GetPath(), "go.mod") {
			return true, nil
		}
	}

	return false, nil
}

func (f *finder) getUserRepos(ctx context.Context) ([]*github.Repository, error) {
	opt := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: 30},
		Affiliation: "owner",
	}
	var list []*github.Repository
	for {
		repos, resp, err := f.gh.Repositories.List(ctx, f.config.owner, opt)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		if f.config.regexp == nil {
			list = append(list, repos...)
		} else {
			for _, repo := range repos {
				if f.config.regexp.Match([]byte(repo.GetName())) {
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

func (f *finder) getOrgRepos(ctx context.Context) ([]*github.Repository, error) {
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 30},
	}
	var list []*github.Repository
	for {
		repos, resp, err := f.gh.Repositories.ListByOrg(ctx, f.config.owner, opt)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		if f.config.regexp == nil {
			list = append(list, repos...)
		} else {
			for _, repo := range repos {
				if f.config.regexp.Match([]byte(repo.GetName())) {
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

func (f *finder) getRepos(ctx context.Context) ([]*github.Repository, error) {
	owner, _, err := f.gh.Users.Get(ctx, f.config.owner)
	if err != nil {
		return nil, fmt.Errorf("can't read owner information: %s", err)
	}

	var repos []*github.Repository
	switch t := owner.GetType(); t {
	case "User":
		repos, err = f.getUserRepos(ctx)
	case "Organization":
		repos, err = f.getOrgRepos(ctx)
	default:
		err = fmt.Errorf("unknown owner type %s", t)
	}

	return repos, err
}
