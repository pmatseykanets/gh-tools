package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitHTTP "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v32/github"
	"github.com/pmatseykanets/gh-tools/auth"
	gh "github.com/pmatseykanets/gh-tools/github"
	"github.com/pmatseykanets/gh-tools/terminal"
	"github.com/pmatseykanets/gh-tools/version"
	"golang.org/x/oauth2"
)

func usage() {
	usage := `Automate PR creation across GitHub repositories

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
  -shell=           The shell to use to run the script
  -title=           The PR title
  -token            Prompt for an Access Token
  -version          Print the version and exit
`
	fmt.Printf("gh-pr version %s\n", version.Version)
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
	repoRegexp   *regexp.Regexp // The pattern to match respository names.
	branch       string         // The branch name if different from the default.
	desc         string         // The PR description.
	reviewers    []string       // The GitHub user login to request the PR review from.
	assignees    []string       // The GitHub user login to assign the PR to.
	script       string         // The body of the script.
	shell        string         // The shell to use to run the script.
	title        string         // The PR title.
	token        bool           // Propmt for an access token.
	noPrivate    bool           // Don't include private repositories.
	noPublic     bool           // Don't include public repositories.
	noFork       bool           // Don't include fork repositories.
	noRepoRegexp *regexp.Regexp // The pattern to reject repository names.
}

type prmaker struct {
	gh      *github.Client
	ghToken string
	config  config
	stdout  io.WriteCloser
	stderr  io.WriteCloser
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

	config := config{
		shell: "bash",
	}

	var (
		showVersion, showHelp    bool
		repo, noRepo, scriptFile string
		review, assign           stringList
		err                      error
	)
	flag.Var(&assign, "assign", "The GitHub user login to assign the PR to")
	flag.StringVar(&config.branch, "branch", "", "The PR branch name")
	flag.StringVar(&config.desc, "desc", "", "The PR description")
	flag.BoolVar(&showHelp, "help", showHelp, "Print this information and exit")
	flag.BoolVar(&config.noFork, "no-fork", config.noFork, "Don't include fork repositories")
	flag.BoolVar(&config.noPrivate, "no-private", config.noPrivate, "Don't include private repositories")
	flag.BoolVar(&config.noPublic, "no-public", config.noPublic, "Don't include public repositories")
	flag.StringVar(&noRepo, "no-repo", "", "The pattern to reject repository names")
	flag.StringVar(&repo, "repo", "", "The pattern to match repository names")
	flag.Var(&review, "review", "The GitHub user login to request the PR review from")
	flag.StringVar(&config.script, "script", "", "The script to apply PR changes")
	flag.StringVar(&scriptFile, "script-file", "", "Read the script from a file")
	flag.StringVar(&config.shell, "shell", config.shell, "The shell to use to run the script")
	flag.StringVar(&config.title, "title", "", "The PR title")
	flag.BoolVar(&config.token, "token", config.token, "Prompt for Access Token")
	flag.BoolVar(&showVersion, "version", showVersion, "Print version and exit")
	flag.Usage = usage
	flag.Parse()

	if showHelp {
		usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("gh-pr version %s\n", version.Version)
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

	if config.branch == "" {
		return config, fmt.Errorf("branch is required")
	}

	if config.shell == "" {
		return config, fmt.Errorf("shell is required")
	}

	if config.script == "" && scriptFile != "" {
		contents, err := ioutil.ReadFile(scriptFile)
		if err != nil {
			return config, fmt.Errorf("can't read script file %s: %s", scriptFile, err)
		}
		config.script = string(contents)
	}
	if config.script == "" {
		return config, fmt.Errorf("script is required")
	}

	if config.title == "" {
		return config, fmt.Errorf("title is required")
	}

	if len(review) > 0 {
		seen := map[string]struct{}{}
		for _, v := range []string(review) {
			v = strings.ToLower(strings.TrimSpace(v))
			seen[v] = struct{}{}
		}
		unique := make([]string, len(seen))
		for v := range seen {
			unique = append(unique, v)
		}
		if len(unique) > 0 {
			config.reviewers = unique
		}
	}

	if len(assign) > 0 {
		seen := map[string]struct{}{}
		for _, v := range []string(assign) {
			v = strings.ToLower(strings.TrimSpace(v))
			seen[v] = struct{}{}
		}
		unique := make([]string, len(seen))
		for v := range seen {
			unique = append(unique, v)
		}
		if len(unique) > 0 {
			config.assignees = unique
		}
	}

	if repo != "" {
		if config.repoRegexp, err = regexp.Compile(repo); err != nil {
			return config, fmt.Errorf("invalid repo pattern: %s", err)
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

	prmaker := &prmaker{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	prmaker.config, err = readConfig()
	if err != nil {
		return err
	}

	var token string
	if prmaker.config.token {
		token, _ = terminal.PasswordPrompt("Access Token: ")
	} else {
		token = auth.GetToken()
	}
	if token == "" {
		return fmt.Errorf("access token is required")
	}

	prmaker.ghToken = token

	prmaker.gh = github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)))

	return prmaker.create(ctx)
}

func (p *prmaker) create(ctx context.Context) error {
	scriptFile, err := ioutil.TempFile("", "gh-pr-script")
	if err != nil {
		return fmt.Errorf("can't create temp file: %s", err)
	}
	scriptFile.WriteString(p.config.script)
	defer func() {
		scriptFile.Close()
		os.Remove(scriptFile.Name()) // Clean up.
	}()

	repos, err := gh.NewRepoFinder(p.gh).Find(ctx, gh.RepoFilter{
		Owner:        p.config.owner,
		Repo:         p.config.repo,
		RepoRegexp:   p.config.repoRegexp,
		Archived:     false,
		NoPrivate:    p.config.noPrivate,
		NoPublic:     p.config.noPublic,
		NoFork:       p.config.noFork,
		NoRepoRegexp: p.config.noRepoRegexp,
	})
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		fmt.Fprintln(p.stdout, "No matching repositories")
		return nil
	}

	// Validate reviewers.
	for _, login := range p.config.reviewers {
		_, resp, err := p.gh.Users.Get(ctx, login)
		if err != nil {
			if resp.StatusCode == http.StatusNotFound {
				return fmt.Errorf("reviewer %s doesn't exist", login)
			}
			return fmt.Errorf("can't get reviewer %s: %s", login, err)
		}
	}
	// Validate assignees.
	for _, login := range p.config.assignees {
		_, resp, err := p.gh.Users.Get(ctx, login)
		if err != nil {
			if resp.StatusCode == http.StatusNotFound {
				return fmt.Errorf("assignee %s doesn't exist", login)
			}
			return fmt.Errorf("can't get assignee %s: %s", login, err)
		}
	}

	var (
		repo *github.Repository
		prNo int
	)
	for _, repo = range repos {
		// Check if the remote branch already exists.
		_, resp, err := p.gh.Repositories.GetBranch(ctx, p.config.owner, repo.GetName(), p.config.branch)
		switch err {
		case nil:
			pullURL := ""
			pull, err := p.getPullForBranch(ctx, repo, p.config.branch)
			if err == nil {
				pullURL = pull.GetHTMLURL()
			}
			fmt.Fprintln(p.stderr, repo.GetFullName(), "The remote branch already exists", pullURL)
			continue
		default:
			if resp != nil && resp.StatusCode != http.StatusNotFound {
				return fmt.Errorf("%s: error checking branch: %s", repo.GetFullName(), err)
			}
		}

		fmt.Fprint(p.stderr, repo.GetFullName())

		err = p.apply(ctx, repo, scriptFile.Name())
		if err != nil {
			if err == errNoChanges {
				fmt.Fprintln(p.stdout, " no changes")
				continue
			}
			fmt.Fprintln(p.stdout)
			return err
		}

		pr, _, err := p.gh.PullRequests.Create(ctx, p.config.owner, repo.GetName(), &github.NewPullRequest{
			Title: &p.config.title,
			Head:  &p.config.branch,
			Base:  repo.DefaultBranch,
			Body:  &p.config.desc,
		})
		if err != nil {
			fmt.Fprintln(p.stdout)
			return fmt.Errorf("%s: error creating a PR: %s", repo.GetFullName(), err)
		}

		prNo = pr.GetNumber()
		if len(p.config.reviewers) > 0 {
			_, _, err = p.gh.PullRequests.RequestReviewers(ctx, p.config.owner, repo.GetName(), prNo, github.ReviewersRequest{
				Reviewers: p.config.reviewers,
			})
			if err != nil {
				fmt.Fprintln(p.stdout)
				fmt.Fprintf(p.stderr, "%s: error requesting a PR review: %s\n", repo.GetFullName(), err)
			}
		}

		if len(p.config.assignees) > 0 {
			_, _, err = p.gh.Issues.AddAssignees(ctx, p.config.owner, repo.GetName(), prNo, p.config.assignees)
			if err != nil {
				fmt.Fprintln(p.stdout)
				fmt.Fprintf(p.stderr, "%s: error assigning the PR: %s\n", repo.GetFullName(), err)
			}
		}

		fmt.Println("", pr.GetHTMLURL())
	}

	return nil
}

func (p *prmaker) getPullForBranch(ctx context.Context, repo *github.Repository, branch string) (*github.PullRequest, error) {
	var (
		pulls []*github.PullRequest
		resp  *github.Response
		err   error
		opts  = &github.PullRequestListOptions{ListOptions: github.ListOptions{PerPage: 100}}
	)
	for {
		pulls, resp, err = p.gh.PullRequests.List(ctx, p.config.owner, repo.GetName(), opts)
		if err != nil {
			return nil, fmt.Errorf("%s: can't read pull requests: %s", repo.GetName(), err)
		}

		for _, pull := range pulls {
			if pull.GetHead().GetRef() == branch {
				return pull, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return nil, nil
}

var errNoChanges = fmt.Errorf("no changes were made")

func (p *prmaker) apply(ctx context.Context, repo *github.Repository, scriptPath string) error {
	dir, err := ioutil.TempDir("", "gh-pr")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) // Clean up.

	auth := &gitHTTP.BasicAuth{
		Username: "user", // Should be a non-empty string.
		Password: p.ghToken,
	}

	// git clone --depth=1.
	gitRepo, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:   repo.GetCloneURL(),
		Auth:  auth,
		Depth: 1,
	})
	if err != nil {
		return fmt.Errorf("%s: git clone error: %s", repo.GetFullName(), err)
	}

	wrkTree, err := gitRepo.Worktree()
	if err != nil {
		return fmt.Errorf("%s: git worktree error: %s", repo.GetFullName(), err)
	}

	// git checkout -b branch.
	headRef, err := gitRepo.Head()
	if err != nil {
		return fmt.Errorf("%s: git show-ref error: %s", repo.GetFullName(), err)
	}
	err = wrkTree.Checkout(&git.CheckoutOptions{
		Hash:   headRef.Hash(),
		Branch: plumbing.ReferenceName("refs/heads/" + p.config.branch),
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("%s: git checkout error: %s", repo.GetFullName(), err)
	}

	// Run the script with the choosen shell.
	cmd := exec.Command(p.config.shell, scriptPath)
	cmd.Dir = dir
	cmdOut, err := cmd.Output()
	if err != nil {
		p.stderr.Write(cmdOut)
		if eerr, ok := err.(*exec.ExitError); ok {
			p.stderr.Write(eerr.Stderr)
		}
		return fmt.Errorf("%s: failed to apply changes: %s", repo.GetFullName(), err)
	}

	// git add .
	_, err = wrkTree.Add(".")
	if err != nil {
		return fmt.Errorf("%s: git add error: %s", repo.GetFullName(), err)
	}

	// Make sure we have changes to commit.
	gitStatus, err := wrkTree.Status()
	if err != nil {
		return fmt.Errorf("%s: git status error: %s", repo.GetFullName(), err)
	}
	if gitStatus.IsClean() {
		return errNoChanges
	}

	// git commit.
	_, err = wrkTree.Commit(p.config.title, &git.CommitOptions{})
	if err != nil {
		return fmt.Errorf("%s: git commit error: %s", repo.GetFullName(), err)
	}

	// git push.
	err = gitRepo.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
	})
	if err != nil {
		return fmt.Errorf("%s: git push error: %s", repo.GetFullName(), err)
	}

	return nil
}
