package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pmatseykanets/gh-tools/version"
	"golang.org/x/oauth2"
)

func usage() {
	usage := `Walk file hierarchies across GitHub repositories

Usage: gh-find [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository name

Flags:
  -help         Print this information and exit
  -branch       Repository branch name if different from the default
  -grep         The pattern to match the file contents
  -maxdepth     Descend at most n directory levels
  -mindepth     Descend at least n directory levels
  -name         The pattern to match the last component of the pathname
  -no-name      The pattern to reject the last component of the pathname
  -path         The pattern to match the pathname
  -no-path      The pattern to reject the pathname
  -repo         The pattern to match repository names
  -type         File type f - file, d - directory
  -version      Print the version and exit

Environment variables:
  GITHUB_TOKEN  an authentication token for github.com API requests
`
	fmt.Printf("gh-find version %s\n", version.Version)
	fmt.Println(usage)
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Printf("error: %s\n", err)
		os.Exit(1)
	}
}

const (
	typeFile = "f"
	typeDir  = "d"
)

type config struct {
	owner        string
	repo         string
	repoRegexp   *regexp.Regexp
	branch       string
	ftype        string           // File type.
	minDepth     int              // Descend at least n directory levels.
	maxDepth     int              // Descend at most n directory levels.
	nameRegexp   []*regexp.Regexp // The pattern to match the last component of the pathname.
	noNameRegexp []*regexp.Regexp // The pattern to reject the last component of the pathname.
	pathRegexp   []*regexp.Regexp // The pattern to match the pathname.
	noPathRegexp []*regexp.Regexp // The pattern to reject the pathname.
	grepRegexp   *regexp.Regexp   // The pattern to match the contents of matching files.
}

type finder struct {
	gh     *github.Client
	config config
	stdout io.WriteCloser
	stderr io.WriteCloser
}

type stringList []string

func (l *stringList) String() string {
	if l == nil {
		return ""
	}
	return strings.Join(*l, ",")
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func readConfig() (config, error) {
	if len(os.Args) == 0 {
		usage()
		os.Exit(1)
	}

	config := config{}

	var (
		showVersion, showHelp      bool
		grep, repo                 string
		name, path, noName, noPath stringList
		err                        error
	)
	flag.BoolVar(&showHelp, "help", showHelp, "Print this information and exit")
	flag.StringVar(&config.branch, "branch", "", "Repository branch name if different from the default")
	flag.StringVar(&grep, "grep", "", "The pattern to match the file contents")
	flag.IntVar(&config.maxDepth, "maxdepth", 0, "Descend at most n directory levels")
	flag.IntVar(&config.minDepth, "mindepth", 0, "Descend at least n directory levels")
	flag.Var(&name, "name", "The pattern to match the last component of the pathname")
	flag.Var(&noName, "no-name", "The pattern to reject the last component of the pathname")
	flag.Var(&path, "path", "The pattern to match the pathname")
	flag.Var(&noPath, "no-path", "The pattern to reject the pathname")
	flag.StringVar(&repo, "repo", "", "The pattern to match repository names")
	flag.StringVar(&config.ftype, "type", "", "File type f - file, d - directory")
	flag.BoolVar(&showVersion, "version", showVersion, "Print version and exit")
	flag.Usage = usage
	flag.Parse()

	if showHelp {
		usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("gh-find version %s\n", version.Version)
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

	config.nameRegexp = make([]*regexp.Regexp, len(name))
	for i, n := range name {
		if config.nameRegexp[i], err = regexp.Compile(n); err != nil {
			return config, fmt.Errorf("invalid name pattern: %s: %s", n, err)
		}
	}
	config.noNameRegexp = make([]*regexp.Regexp, len(noName))
	for i, n := range noName {
		if config.noNameRegexp[i], err = regexp.Compile(n); err != nil {
			return config, fmt.Errorf("invalid no-name pattern: %s: %s", n, err)
		}
	}

	config.pathRegexp = make([]*regexp.Regexp, len(path))
	for i, n := range path {
		if config.pathRegexp[i], err = regexp.Compile(n); err != nil {
			return config, fmt.Errorf("invalid path pattern: %s: %s", n, err)
		}
	}
	config.noPathRegexp = make([]*regexp.Regexp, len(noPath))
	for i, n := range noPath {
		if config.noPathRegexp[i], err = regexp.Compile(n); err != nil {
			return config, fmt.Errorf("invalid no-path pattern: %s: %s", n, err)
		}
	}

	if repo != "" {
		if config.repoRegexp, err = regexp.Compile(repo); err != nil {
			return config, fmt.Errorf("invalid repo pattern: %s", err)
		}
	}

	switch t := config.ftype; t {
	case "", typeFile, typeDir: // Empty or valid.
	default:
		return config, fmt.Errorf("invalid type: %s", t)
	}

	if grep != "" {
		if config.grepRegexp, err = regexp.Compile(grep); err != nil {
			return config, fmt.Errorf("invalid grep pattern: %s", err)
		}
		config.ftype = typeFile // Implies type file.
	}

	if config.maxDepth < 0 {
		return config, fmt.Errorf("maxdepth value should be positive")
	}
	if config.minDepth < 0 {
		return config, fmt.Errorf("mindepth value should be positive")
	}
	if config.maxDepth > 0 && config.minDepth > 0 && config.maxDepth < config.minDepth {
		return config, fmt.Errorf("mindepth can't be greater than maxdepth")
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
		branch, entryPath, basename string
		level                       int
	)
	// REPOS:
	for _, repo := range repos {
		branch = f.config.branch
		if branch == "" {
			branch = repo.GetDefaultBranch()
		}

		// fmt.Println(repo.GetFullName(), branch)

		tree, resp, err := f.gh.Git.GetTree(ctx, f.config.owner, *repo.Name, branch, true)
		if err != nil {
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusConflict {
				// http.StatusConflict - Git Repository is empty.
				continue
			}
			return err
		}

	nextEntry:
		for _, entry := range tree.Entries {
			entryPath = entry.GetPath()
			level = levels(entryPath)
			if f.config.minDepth > 0 && level < f.config.minDepth {
				continue
			}
			if f.config.maxDepth > 0 && level > f.config.maxDepth {
				continue
			}

			switch f.config.ftype {
			case typeFile:
				if entry.GetType() != "blob" {
					continue
				}
			case typeDir:
				if entry.GetType() != "tree" {
					continue
				}
			}

			// Check for path rejects first.
			if len(f.config.noPathRegexp) > 0 && matchAny(entryPath, f.config.noPathRegexp) {
				continue nextEntry
			}
			// Then check for path matches.
			if len(f.config.pathRegexp) > 0 && !matchAny(entryPath, f.config.pathRegexp) {
				continue nextEntry
			}

			_, basename = path.Split(entryPath)
			// Then check for name rejects.
			if len(f.config.noNameRegexp) > 0 && matchAny(basename, f.config.noNameRegexp) {
				continue nextEntry
			}
			// And finally check for name matches.
			if len(f.config.nameRegexp) > 0 && !matchAny(basename, f.config.nameRegexp) {
				continue nextEntry
			}

			if f.config.grepRegexp != nil && entry.GetType() == "blob" {
				results, err := f.grepContents(ctx, repo, branch, entry)
				if err != nil {
					return err
				}
				for _, match := range results.matches {
					fmt.Println(repo.GetFullName(), entry.GetPath(), match.lineno, match.line)
				}
				continue nextEntry
			}

			fmt.Println(repo.GetFullName(), entry.GetPath())
		}
	}

	return nil
}

func (f *finder) grepContents(ctx context.Context, repo *github.Repository, branch string, entry *github.TreeEntry) (*grepResults, error) {
	if f.config.grepRegexp == nil {
		return nil, nil // There is nothing to do.
	}

	opts := &github.RepositoryContentGetOptions{Ref: branch}
	contents, err := f.gh.Repositories.DownloadContents(ctx, f.config.owner, repo.GetName(), entry.GetPath(), opts)
	if err != nil {
		return nil, err
	}
	defer contents.Close()

	return grep(contents, f.config.grepRegexp)
}

func (f *finder) getSingleRepo(ctx context.Context) (*github.Repository, error) {
	repo, _, err := f.gh.Repositories.Get(ctx, f.config.owner, f.config.repo)
	if err != nil {
		return nil, fmt.Errorf("can't read repository: %s", err)
	}

	return repo, nil
}

func (f *finder) getUserRepos(ctx context.Context) ([]*github.Repository, error) {
	opt := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: 30},
		Affiliation: "owner",
	}
	var (
		list, repos []*github.Repository
		resp        *github.Response
		err         error
	)
	for {
		repos, resp, err = f.gh.Repositories.List(ctx, f.config.owner, opt)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		if f.config.repoRegexp == nil {
			list = append(list, repos...)
		} else {
			for _, repo := range repos {
				if f.config.repoRegexp.MatchString(repo.GetName()) {
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
	var (
		list, repos []*github.Repository
		resp        *github.Response
		err         error
	)
	for {
		repos, resp, err = f.gh.Repositories.ListByOrg(ctx, f.config.owner, opt)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		if f.config.repoRegexp == nil {
			list = append(list, repos...)
		} else {
			for _, repo := range repos {
				if f.config.repoRegexp.Match([]byte(repo.GetName())) {
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

	// A single repository.
	if f.config.repo != "" {
		repo, err := f.getSingleRepo(ctx)
		if err != nil {
			return nil, err
		}
		return []*github.Repository{repo}, nil
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

func levels(path string) int {
	return len(path) - len(strings.ReplaceAll(path, "/", "")) + 1
}

func matchAny(s string, regexes []*regexp.Regexp) bool {
	for _, regex := range regexes {
		if regex.MatchString(s) {
			return true
		}
	}

	return false
}
