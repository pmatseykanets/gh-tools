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
	"github.com/pmatseykanets/gh-tools/auth"
	gh "github.com/pmatseykanets/gh-tools/github"
	"github.com/pmatseykanets/gh-tools/size"
	"github.com/pmatseykanets/gh-tools/terminal"
	"github.com/pmatseykanets/gh-tools/version"
	"golang.org/x/oauth2"
)

func usage() {
	usage := `Walk file hierarchies across GitHub repositories

Usage: gh-find [flags] [owner][/repo]
  owner         Repository owner (user or organization)
  repo          Repository name

Flags:
  -archived          Include archived repositories
  -help, h           Print this information and exit
  -branch=           The branch name if different from the default
  -grep=             The pattern to match the file contents. Implies
                      -type f
  -list-details      List details (file type, author, size, last commit date)
  -max-depth         Descend at most n directory levels
  -max-grep-results= Limit the number of grep results
  -max-repo-results= Limit the number of matched entries per repository
  -max-results=      Limit the number of matched entries
  -min-depth=        Descend at least n directory levels
  -name=             The pattern to match the last component of the pathname
  -no-fork           Don't include fork repositories
  -no-grep=          The pattern to reject the file contents. Implies
                       -type f
  -no-matches        List repositories with no matches. Implies
                       -max-results 0
                       -max-grep-results 1
                       -max-repo-results 1
  -no-name=          The pattern to reject the last component of the pathname
  -no-path=          The pattern to reject the pathname
  -no-private        Don't include private repositories
  -no-public         Don't include public repositories
  -path=             The pattern to match the pathname
  -repo=             The pattern to match repository names
  -size=             Limit results based on the file size [+-]<d><u>
  -token             Prompt for an Access Token
  -type=             The entry type f - file, d - directory
  -version           Print the version and exit
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

type sizePredicate struct {
	op    int   // <0 - less than, 0 - equals, >0 greater than
	value int64 // Size in bytes
}

func (p *sizePredicate) match(value int64) bool {
	switch p.op {
	case 0:
		return value == p.value
	case 1:
		return value >= p.value
	default:
		return value <= p.value
	}
}

type config struct {
	owner          string
	repo           string
	repoRegexp     *regexp.Regexp   // The pattern to match respository names.
	branch         string           // The branch name if different from the default.
	ftype          string           // The entry type f - file, d - directory.
	minDepth       int              // Descend at least n directory levels.
	maxDepth       int              // Descend at most n directory levels.
	maxResults     int              // Limit the number of matched entries.
	maxRepoResults int              // Limit the number of matched entries per repository.
	nameRegexp     []*regexp.Regexp // The pattern to match the last component of the pathname.
	noNameRegexp   []*regexp.Regexp // The pattern to reject the last component of the pathname.
	pathRegexp     []*regexp.Regexp // The pattern to match the pathname.
	noPathRegexp   []*regexp.Regexp // The pattern to reject the pathname.
	grepRegexp     *regexp.Regexp   // The pattern to match the contents of matching files.
	noGrepRegexp   *regexp.Regexp   // The pattern to reject the file contents.
	token          bool             // Propmt for an access token.
	size           *sizePredicate   // Limit results based on the file size [+-]<d><u>.
	noMatches      bool             // List repositories with no matches.
	maxGrepResults int              // Limit the number of grep results.
	listDetails    bool             // List details.
	archived       bool             // Include archived repositories.
	noPrivate      bool             // Don't include private repositories.
	noPublic       bool             // Don't include public repositories.
	noFork         bool             // Don't include fork repositories.
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
		grep, noGrep, repo, fsize  string
		name, path, noName, noPath stringList
		err                        error
	)
	flag.BoolVar(&config.archived, "archived", config.archived, "Include archived repositories")
	flag.StringVar(&config.branch, "branch", "", "The branch name if different from the default")
	flag.BoolVar(&showHelp, "help", false, "Print this information and exit")
	flag.StringVar(&grep, "grep", "", "The pattern to match the file contents")
	flag.BoolVar(&config.listDetails, "list-details", config.listDetails, "List details (file type, author, size, last commit date)")
	flag.IntVar(&config.maxDepth, "max-depth", 0, "Descend at most n directory levels")
	flag.IntVar(&config.maxGrepResults, "max-grep-results", 0, "Limit the number of grep results.")
	flag.IntVar(&config.maxResults, "max-results", 0, "Limit the number of matched entries")
	flag.IntVar(&config.maxRepoResults, "max-repo-results", 0, "Limit the number of matched entries per repository")
	flag.IntVar(&config.minDepth, "min-depth", 0, "Descend at least n directory levels")
	flag.Var(&name, "name", "The pattern to match the last component of the pathname")
	flag.BoolVar(&config.noFork, "no-fork", config.noFork, "Don't include fork repositories")
	flag.StringVar(&noGrep, "no-grep", "", "The pattern to reject the file contents")
	flag.BoolVar(&config.noMatches, "no-matches", config.noMatches, "List repositories with no matches")
	flag.Var(&noName, "no-name", "The pattern to reject the last component of the pathname")
	flag.Var(&noPath, "no-path", "The pattern to reject the pathname")
	flag.BoolVar(&config.noPrivate, "no-private", config.noPrivate, "Don't include private repositories")
	flag.BoolVar(&config.noPublic, "no-public", config.noPublic, "Don't include public repositories")
	flag.Var(&path, "path", "The pattern to match the pathname")
	flag.StringVar(&repo, "repo", "", "The pattern to match repository names")
	flag.StringVar(&fsize, "size", "", "Limit results based on the file size [+-]<d><u>")
	flag.BoolVar(&config.token, "token", config.token, "Prompt for Access Token")
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

	if config.noPrivate && config.noPublic {
		return config, fmt.Errorf("no-private and no-public are mutually exclusive")
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
		config.ftype = typeFile // Implies file type.
	}
	if noGrep != "" {
		if config.noGrepRegexp, err = regexp.Compile(noGrep); err != nil {
			return config, fmt.Errorf("invalid no-grep pattern: %s", err)
		}
		config.ftype = typeFile // Implies file type.
	}

	if config.maxDepth < 0 {
		return config, fmt.Errorf("max-depth should be positive")
	}
	if config.minDepth < 0 {
		return config, fmt.Errorf("min-depth should be positive")
	}
	if config.maxDepth > 0 && config.minDepth > 0 && config.maxDepth < config.minDepth {
		return config, fmt.Errorf("min-depth should be less than max-depth")
	}
	if config.maxResults < 0 {
		return config, fmt.Errorf("max-results should be positive")
	}
	if config.maxRepoResults < 0 {
		return config, fmt.Errorf("max-repo-results should be positive")
	}
	if config.maxGrepResults < 0 {
		return config, fmt.Errorf("max-grep-results should be positive")
	}

	if fsize != "" {
		p := &sizePredicate{}
		switch fsize[0] {
		case '+':
			p.op = 1
		case '-':
			p.op = -1
		}
		offset := 0
		if p.op != 0 {
			offset = 1
		}
		value, err := size.Parse(fsize[offset:])
		if err != nil {
			return config, fmt.Errorf("invalid size %s", fsize)
		}
		p.value = value
		config.size = p
		config.ftype = typeFile // Implies file type.
	}

	if config.noMatches {
		// Implies no limit on max overall results.
		config.maxResults = 0
		// And there is no reason to look futher at the repo level
		// if we have at least one entry match.
		config.maxRepoResults = 1
		// Or a at least one grep match.
		config.maxGrepResults = 1
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

	var token string
	if finder.config.token {
		token, _ = terminal.PasswordPrompt("Access Token: ")
	} else {
		token = auth.GetToken()
	}
	if token == "" {
		return fmt.Errorf("access token is required")
	}

	finder.gh = github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)))

	return finder.find(ctx)
}

func (f *finder) find(ctx context.Context) error {
	repos, err := gh.NewRepoFinder(f.gh).Find(ctx, gh.RepoFilter{
		Owner:      f.config.owner,
		Repo:       f.config.repo,
		RepoRegexp: f.config.repoRegexp,
		Archived:   f.config.archived,
		NoPrivate:  f.config.noPrivate,
		NoPublic:   f.config.noPublic,
		NoFork:     f.config.noFork,
	})
	if err != nil {
		return err
	}

	var (
		branch, entryPath, basename string
		level, matched, repoMatched int
		repo, prevRepo              *github.Repository
	)
nextRepo:
	for _, repo = range repos {
		if prevRepo != nil && f.config.noMatches && repoMatched == 0 {
			fmt.Fprintln(f.stdout, prevRepo.GetFullName())
		}
		prevRepo = repo
		repoMatched = 0 // Reset per repository counter.

		// Check the number of overall matched entries.
		if f.config.maxResults > 0 && matched >= f.config.maxResults {
			return nil
		}

		branch = f.config.branch
		if branch == "" {
			branch = repo.GetDefaultBranch()
		}

		tree, resp, err := f.gh.Git.GetTree(ctx, f.config.owner, repo.GetName(), branch, true)
		if err != nil {
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusConflict {
				// http.StatusConflict - Git Repository is empty.
				continue
			}
			return err
		}

		if tree.GetTruncated() {
			fmt.Fprintf(f.stderr, "WARNING: results were truncated for %s", repo.GetFullName())
		}

	nextEntry:
		for _, entry := range tree.Entries {
			// Check the number of overall matched entries.
			if f.config.maxResults > 0 && matched >= f.config.maxResults {
				return nil
			}
			// Check the number of per repository matched entries.
			if f.config.maxRepoResults > 0 && repoMatched >= f.config.maxRepoResults {
				continue nextRepo
			}

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

			// Check size.
			if f.config.size != nil && !f.config.size.match(int64(entry.GetSize())) {
				continue nextEntry
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
			// Check if we need to reject based on the contents of the file.
			if f.config.noGrepRegexp != nil && entry.GetType() == "blob" {
				results, err := f.grepContents(ctx, repo, branch, entry, 1)
				if err != nil {
					return err
				}
				if len(results.matches) > 0 {
					continue nextEntry
				}
			}

			if f.config.grepRegexp != nil && entry.GetType() == "blob" {
				results, err := f.grepContents(ctx, repo, branch, entry, f.config.maxGrepResults)
				if err != nil {
					return err
				}

				if len(results.matches) > 0 {
					matched++
					repoMatched++
				}

				if !f.config.noMatches {
					for _, match := range results.matches {
						fmt.Fprintln(f.stdout, repo.GetFullName(), entry.GetPath(), match.lineno, match.line)
					}
				}
				continue nextEntry
			}

			matched++
			repoMatched++
			if !f.config.noMatches {
				if !f.config.listDetails {
					fmt.Fprintln(f.stdout, repo.GetFullName(), entry.GetPath())
					continue nextEntry
				}

				commit, err := f.getLastCommit(ctx, repo, branch, entry)
				if err != nil {
					return err
				}
				fmt.Fprintln(f.stdout, repo.GetFullName(), entryType(entry),
					commit.Author.GetLogin(), entry.GetSize(),
					commit.Commit.Author.GetDate().Format("Jan 2 15:04:05 2006"),
					entry.GetPath(),
				)
			}
		}
	}
	if prevRepo != nil && f.config.noMatches && repoMatched == 0 {
		fmt.Fprintln(f.stdout, prevRepo.GetFullName())
	}

	return nil
}

func entryType(e *github.TreeEntry) string {
	if e == nil {
		return ""
	}

	switch e.GetType() {
	case "tree":
		return "d"
	case "blob":
		return "f"
	default:
		return ""
	}
}

func (f *finder) getLastCommit(ctx context.Context, repo *github.Repository, branch string, entry *github.TreeEntry) (*github.RepositoryCommit, error) {
	opts := &github.CommitsListOptions{
		SHA:  branch,
		Path: entry.GetPath(),
		ListOptions: github.ListOptions{
			Page:    1,
			PerPage: 1,
		},
	}
	commits, resp, err := f.gh.Repositories.ListCommits(ctx, f.config.owner, repo.GetName(), opts)
	if err != nil {
		return nil, err
	}
	_ = resp

	if len(commits) == 0 {
		return nil, nil
	}

	return commits[0], nil
}

func (f *finder) grepContents(ctx context.Context, repo *github.Repository, branch string, entry *github.TreeEntry, limit int) (*grepResults, error) {
	if f.config.grepRegexp == nil {
		return nil, nil // There is nothing to do.
	}

	opts := &github.RepositoryContentGetOptions{Ref: branch}
	contents, err := f.gh.Repositories.DownloadContents(ctx, f.config.owner, repo.GetName(), entry.GetPath(), opts)
	if err != nil {
		return nil, err
	}
	defer contents.Close()

	return grep(contents, f.config.grepRegexp, limit)
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
