package main

import (
	"context"
	"errors"
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
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
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
  -commit-message=  The commit message
  -desc=            The PR description
  -no-fork          Don't include fork repositories
  -no-private       Don't include private repositories
  -no-public        Don't include public repositories
  -no-repo=         The pattern to reject repository names
  -patch            Apply changes to the existing PR
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
	owner         string
	repo          string
	repoRegexp    *regexp.Regexp // The pattern to match respository names.
	branch        string         // The branch name if different from the default.
	desc          string         // The PR description.
	reviewers     []string       // The GitHub user login to request the PR review from.
	assignees     []string       // The GitHub user login to assign the PR to.
	script        string         // The body of the script.
	shell         string         // The shell to use to run the script.
	title         string         // The PR title.
	token         bool           // Propmt for an access token.
	noPrivate     bool           // Don't include private repositories.
	noPublic      bool           // Don't include public repositories.
	noFork        bool           // Don't include fork repositories.
	noRepoRegexp  *regexp.Regexp // The pattern to reject repository names.
	patch         bool           // Apply changes to the existing PR
	commitMessage string         // The commit message
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
	flag.BoolVar(&config.patch, "patch", config.patch, "Apply changes to the existing PR")
	flag.Var(&assign, "assign", "The GitHub user login to assign the PR to")
	flag.StringVar(&config.commitMessage, "commit-message", "", "The commit message")
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

	if config.title == "" && config.commitMessage == "" {
		return config, fmt.Errorf("either title or commit-message must be provided")
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
		repo  *github.Repository
		prNo  int
		pr    *github.PullRequest
		prURL string
	)
	for _, repo = range repos {
		fmt.Fprint(p.stderr, repo.GetFullName())

		// Check if the remote branch already exists.
		_, resp, err := p.gh.Repositories.GetBranch(ctx, p.config.owner, repo.GetName(), p.config.branch)
		switch err {
		case nil:
			prURL = ""
			pr, err = p.getPullForBranch(ctx, repo, p.config.branch)
			if err == nil {
				prURL = pr.GetHTMLURL()
			}
			if p.config.patch { // Adding to the exisitng PR.
				if pr != nil {
					fmt.Fprint(p.stdout, " ", prURL)
				} else {
					fmt.Fprintln(p.stdout, " no PR found")
					continue
				}
			} else {
				fmt.Fprintln(p.stdout, " the remote branch already exists ", prURL)
				continue
			}
		default:
			if p.config.patch && resp != nil && resp.StatusCode == http.StatusNotFound {
				fmt.Fprintln(p.stdout, " branch not found")
				continue
			}

			if resp != nil && resp.StatusCode != http.StatusNotFound {
				fmt.Fprintln(p.stdout)
				return fmt.Errorf("%s: error checking branch: %s", repo.GetFullName(), err)
			}
		}

		err = p.apply(ctx, repo, scriptFile.Name())
		switch {
		case err == nil:
		case errors.Is(err, errNoChanges):
			fmt.Fprint(p.stdout, " no changes")
			if !p.config.patch {
				fmt.Fprintln(p.stdout)
				continue
			}
		case errors.Is(err, transport.ErrEmptyRemoteRepository):
			fmt.Fprintln(p.stdout, " empty repository")
			continue
		default:
			fmt.Fprintln(p.stdout)
			return err
		}

		if !p.config.patch {
			// Create a new PR when not in the patch mode.
			pr, _, err = p.gh.PullRequests.Create(ctx, p.config.owner, repo.GetName(), &github.NewPullRequest{
				Title: &p.config.title,
				Head:  &p.config.branch,
				Base:  repo.DefaultBranch,
				Body:  &p.config.desc,
			})
			if err != nil {
				fmt.Fprintln(p.stdout)
				return fmt.Errorf("%s: error creating a PR: %s", repo.GetFullName(), err)
			}

			fmt.Fprint(p.stdout, " ", pr.GetHTMLURL())
		}

		prNo = pr.GetNumber()

		// Add or update reviewers.
		addReviewers := p.config.reviewers
		var deleteReviewers []string
		if p.config.patch && len(addReviewers) > 0 {
			reviewers, _, err := p.gh.PullRequests.ListReviewers(ctx, p.config.owner, repo.GetName(), prNo, nil)
			if err != nil {
				fmt.Fprintln(p.stdout)
				fmt.Fprintf(p.stderr, "%s: error requesting PR reviewers: %s\n", repo.GetFullName(), err)
				continue
			}
			for i, reviewer := range reviewers.Users {
				if contains(addReviewers, reviewer.GetLogin()) {
					addReviewers = append(addReviewers[:i], addReviewers[i+1:]...)
				} else {
					deleteReviewers = append(deleteReviewers, reviewer.GetLogin())
				}
			}
		}
		if len(addReviewers) > 0 {
			_, _, err = p.gh.PullRequests.RequestReviewers(ctx, p.config.owner, repo.GetName(), prNo, github.ReviewersRequest{
				Reviewers: addReviewers,
			})
			if err != nil {
				fmt.Fprintln(p.stdout)
				fmt.Fprintf(p.stderr, "%s: error requesting a PR review: %s\n", repo.GetFullName(), err)
			}
		}
		if len(deleteReviewers) > 0 {
			_, err = p.gh.PullRequests.RemoveReviewers(ctx, p.config.owner, repo.GetName(), prNo, github.ReviewersRequest{
				Reviewers: deleteReviewers,
			})
			if err != nil {
				fmt.Fprintln(p.stdout)
				fmt.Fprintf(p.stderr, "%s: error removing reviewers: %s\n", repo.GetFullName(), err)
			}
		}

		// Add or update assignees.
		addAssignees := p.config.assignees
		var deleteAssignees []string
		if p.config.patch && len(addAssignees) > 0 {
			issue, _, err := p.gh.Issues.Get(ctx, p.config.owner, repo.GetName(), prNo)
			if err != nil {
				fmt.Fprintln(p.stdout)
				fmt.Fprintf(p.stderr, "%s: error retrieving PR: %s\n", repo.GetFullName(), err)
				continue
			}

			for i, assignee := range issue.Assignees {
				if contains(addAssignees, assignee.GetLogin()) {
					addAssignees = append(addAssignees[:i], addAssignees[i+1:]...)
				} else {
					deleteAssignees = append(deleteAssignees, assignee.GetLogin())
				}
			}
		}
		if len(addAssignees) > 0 {
			_, _, err = p.gh.Issues.AddAssignees(ctx, p.config.owner, repo.GetName(), prNo, addAssignees)
			if err != nil {
				fmt.Fprintln(p.stdout)
				fmt.Fprintf(p.stderr, "%s: error assigning the PR: %s\n", repo.GetFullName(), err)
			}
		}
		if len(deleteAssignees) > 0 {
			_, _, err = p.gh.Issues.RemoveAssignees(ctx, p.config.owner, repo.GetName(), prNo, deleteAssignees)
			if err != nil {
				fmt.Fprintln(p.stdout)
				fmt.Fprintf(p.stderr, "%s: error removing assignees: %s\n", repo.GetFullName(), err)
			}
		}

		// Update title and/or body of the PR.
		if p.config.patch {
			var (
				updatePR bool
				updates  github.PullRequest
			)
			if p.config.title != "" && updates.Title != &p.config.title {
				updates.Title = &p.config.title
				updatePR = true
			}
			if p.config.desc != "" && updates.Body != &p.config.desc {
				updates.Body = &p.config.desc
				updatePR = true
			}

			if updatePR {
				pr, _, err = p.gh.PullRequests.Edit(ctx, p.config.owner, repo.GetName(), prNo, &updates)
				if err != nil {
					fmt.Fprintln(p.stdout)
					fmt.Fprintf(p.stderr, "%s: error updating PR: %s\n", repo.GetFullName(), err)
				}
			}
		}

		fmt.Fprintln(p.stdout)
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

	// git clone [--depth=1].
	cloneOptions := &git.CloneOptions{
		URL:  repo.GetCloneURL(),
		Auth: auth,
	}
	if !p.config.patch {
		cloneOptions.Depth = 1
	}
	gitRepo, err := git.PlainCloneContext(ctx, dir, false, cloneOptions)
	if err != nil {
		return fmt.Errorf("%s: git clone error: %w", repo.GetFullName(), err)
	}

	wrkTree, err := gitRepo.Worktree()
	if err != nil {
		return fmt.Errorf("%s: git worktree error: %w", repo.GetFullName(), err)
	}

	// git checkout [-b] branch.
	checkoutOptions := &git.CheckoutOptions{
		Branch: plumbing.ReferenceName("refs/heads/" + p.config.branch),
	}
	if !p.config.patch {
		headRef, err := gitRepo.Head()
		if err != nil {
			return fmt.Errorf("%s: git show-ref error: %w", repo.GetFullName(), err)
		}
		checkoutOptions.Hash = headRef.Hash()
		checkoutOptions.Create = true
	} else {
		err = gitRepo.Fetch(&git.FetchOptions{
			RefSpecs: []gitConfig.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
			Auth:     auth,
		})
		if err != nil {
			return fmt.Errorf("%s: git fetch error: %w", repo.GetFullName(), err)
		}
		checkoutOptions.Force = true
	}

	err = wrkTree.Checkout(checkoutOptions)
	if err != nil {
		return fmt.Errorf("%s: git checkout error: %w", repo.GetFullName(), err)
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
		return fmt.Errorf("%s: failed to apply changes: %w", repo.GetFullName(), err)
	}

	// git add .
	_, err = wrkTree.Add(".")
	if err != nil {
		return fmt.Errorf("%s: git add error: %w", repo.GetFullName(), err)
	}

	// Make sure we have changes to commit.
	gitStatus, err := wrkTree.Status()
	if err != nil {
		return fmt.Errorf("%s: git status error: %w", repo.GetFullName(), err)
	}
	if gitStatus.IsClean() {
		return errNoChanges
	}

	// git commit.
	commitMessage := p.config.commitMessage
	if commitMessage == "" {
		commitMessage = p.config.title
		if p.config.desc != "" {
			commitMessage += "\n\n" + p.config.desc
		}
	}
	_, err = wrkTree.Commit(commitMessage, &git.CommitOptions{})
	if err != nil {
		return fmt.Errorf("%s: git commit error: %w", repo.GetFullName(), err)
	}

	// git push.
	err = gitRepo.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
	})
	if err != nil {
		return fmt.Errorf("%s: git push error: %w", repo.GetFullName(), err)
	}

	return nil
}
