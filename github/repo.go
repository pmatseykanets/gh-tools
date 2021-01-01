package github

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/go-github/v32/github"
)

type RepoFinder struct {
	Client     *github.Client
	Owner      string
	Repo       string
	RepoRegexp *regexp.Regexp
}

func (f *RepoFinder) Find(ctx context.Context) ([]*github.Repository, error) {
	owner, _, err := f.Client.Users.Get(ctx, f.Owner)
	if err != nil {
		return nil, fmt.Errorf("can't read owner information: %s", err)
	}

	// A single repository.
	if f.Repo != "" {
		repo, err := f.singleRepo(ctx)
		if err != nil {
			return nil, err
		}
		return []*github.Repository{repo}, nil
	}

	var repos []*github.Repository
	switch t := owner.GetType(); t {
	case "User":
		repos, err = f.userRepos(ctx)
	case "Organization":
		repos, err = f.orgRepos(ctx)
	default:
		err = fmt.Errorf("unknown owner type %s", t)
	}

	return repos, err
}

func (f *RepoFinder) singleRepo(ctx context.Context) (*github.Repository, error) {
	repo, _, err := f.Client.Repositories.Get(ctx, f.Owner, f.Repo)
	if err != nil {
		return nil, fmt.Errorf("can't read repository: %s", err)
	}

	return repo, nil
}

func (f *RepoFinder) userRepos(ctx context.Context) ([]*github.Repository, error) {
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
		repos, resp, err = f.Client.Repositories.List(ctx, f.Owner, opt)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		if f.RepoRegexp == nil {
			list = append(list, repos...)
		} else {
			for _, repo := range repos {
				if f.RepoRegexp.MatchString(repo.GetName()) {
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

func (f *RepoFinder) orgRepos(ctx context.Context) ([]*github.Repository, error) {
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 30},
	}
	var (
		list, repos []*github.Repository
		resp        *github.Response
		err         error
	)
	for {
		repos, resp, err = f.Client.Repositories.ListByOrg(ctx, f.Owner, opt)
		if err != nil {
			return nil, fmt.Errorf("can't read repositories: %s", err)
		}

		if f.RepoRegexp == nil {
			list = append(list, repos...)
		} else {
			for _, repo := range repos {
				if f.RepoRegexp.Match([]byte(repo.GetName())) {
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
