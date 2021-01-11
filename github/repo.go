package github

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/go-github/v32/github"
)

// RepoFinder finds GitHub repository given RepoFilter.
type RepoFinder struct {
	Client *github.Client
}

// NewRepoFinder creates a new RepoFinder instance.
func NewRepoFinder(client *github.Client) *RepoFinder {
	return &RepoFinder{Client: client}
}

// RepoFilter represents criteria used to filter repositories.
type RepoFilter struct {
	Owner      string         // The owner name. Can be a user or an organization.
	Repo       string         // The repository name when in single-repo mode.
	RepoRegexp *regexp.Regexp // The pattern to match repository names.
	Archived   bool           // Include archived repositories.
	NoPrivate  bool           // Don't inlucde private repositories.
	NoPublic   bool           // Don't include public repositories.
	NoFork     bool           // Don't include forks.
}

// Find repositories using a given filter.
func (f *RepoFinder) Find(ctx context.Context, filter RepoFilter) ([]*github.Repository, error) {
	if filter.NoPrivate && filter.NoPublic {
		return nil, nil // Nothing to do.
	}

	owner, _, err := f.Client.Users.Get(ctx, filter.Owner)
	if err != nil {
		return nil, fmt.Errorf("can't read owner information: %s", err)
	}

	// A single repository. No other criteria apply.
	if filter.Repo != "" {
		repo, _, err := f.Client.Repositories.Get(ctx, filter.Owner, filter.Repo)
		if err != nil {
			return nil, fmt.Errorf("can't read repository: %s", err)
		}
		return []*github.Repository{repo}, nil
	}

	var repos []*github.Repository
	switch t := owner.GetType(); t {
	case "User":
		repos, err = f.userRepos(ctx, filter)
	case "Organization":
		repos, err = f.orgRepos(ctx, filter)
	default:
		err = fmt.Errorf("unknown owner type %s", t)
	}

	return repos, err
}

var listOptions = github.ListOptions{PerPage: 100}

func (f *RepoFinder) userRepos(ctx context.Context, filter RepoFilter) ([]*github.Repository, error) {
	opts := &github.RepositoryListOptions{
		ListOptions: listOptions,
		Affiliation: "owner",
	}
	var (
		filtered, repos []*github.Repository
		resp            *github.Response
		err             error
	)
	for {
		repos, resp, err = f.Client.Repositories.List(ctx, filter.Owner, opts)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		filtered = append(filtered, apply(repos, filter)...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return filtered, nil
}

func (f *RepoFinder) orgRepos(ctx context.Context, filter RepoFilter) ([]*github.Repository, error) {
	opts := &github.RepositoryListByOrgOptions{ListOptions: listOptions}
	var (
		filtered, repos []*github.Repository
		resp            *github.Response
		err             error
	)
	for {
		repos, resp, err = f.Client.Repositories.ListByOrg(ctx, filter.Owner, opts)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		filtered = append(filtered, apply(repos, filter)...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return filtered, nil
}

func apply(repos []*github.Repository, filter RepoFilter) []*github.Repository {
	var (
		filtered = make([]*github.Repository, len(repos))
		n        int
	)
	for _, repo := range repos {
		if repo.GetArchived() && !filter.Archived {
			continue
		}

		if repo.GetPrivate() {
			if filter.NoPrivate {
				continue
			}
		} else {
			if filter.NoPublic {
				continue
			}
		}

		if repo.GetFork() && filter.NoFork {
			continue
		}

		if filter.RepoRegexp != nil && !filter.RepoRegexp.MatchString(repo.GetName()) {
			continue
		}

		filtered[n] = repo
		n++
	}

	if n == 0 {
		return nil
	}

	return filtered[:n]
}
