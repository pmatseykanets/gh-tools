package github

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/google/go-github/v32/github"
)

func boolp(v bool) *bool {
	return &v
}

func stringp(v string) *string {
	return &v
}

func TestApply(t *testing.T) {
	tests := []struct {
		desc   string
		in     []*github.Repository
		filter RepoFilter
		out    []*github.Repository
	}{
		{
			desc: "all matched",
			in: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar")},
			},
			filter: RepoFilter{},
			out: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar")},
			},
		},
		{
			desc: "foo matched",
			in: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar")},
			},
			filter: RepoFilter{
				RepoRegexp: regexp.MustCompile("foo"),
			},
			out: []*github.Repository{
				{Name: stringp("foo")},
			},
		},
		{
			desc: "no private",
			in: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar"), Private: boolp(true)},
			},
			filter: RepoFilter{
				NoPrivate: true,
			},
			out: []*github.Repository{
				{Name: stringp("foo")},
			},
		},
		{
			desc: "no public",
			in: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar"), Private: boolp(true)},
			},
			filter: RepoFilter{
				NoPublic: true,
			},
			out: []*github.Repository{
				{Name: stringp("bar"), Private: boolp(true)},
			},
		},
		{
			desc: "no fork",
			in: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar"), Fork: boolp(true)},
			},
			filter: RepoFilter{
				NoFork: true,
			},
			out: []*github.Repository{
				{Name: stringp("foo")},
			},
		},
		{
			desc: "no archived by default",
			in: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar"), Archived: boolp(true)},
			},
			filter: RepoFilter{},
			out: []*github.Repository{
				{Name: stringp("foo")},
			},
		},
		{
			desc: "include archived",
			in: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar"), Archived: boolp(true)},
			},
			filter: RepoFilter{
				Archived: true,
			},
			out: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar"), Archived: boolp(true)},
			},
		},
		{
			desc: "no matches",
			in: []*github.Repository{
				{Name: stringp("foo")},
				{Name: stringp("bar")},
			},
			filter: RepoFilter{
				RepoRegexp: regexp.MustCompile("baz"),
			},
		},
		{
			desc:   "empty input",
			filter: RepoFilter{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			if want, got := tt.out, apply(tt.in, tt.filter); !reflect.DeepEqual(want, got) {
				t.Errorf("Expected\n%+v\ngot\n%+v", want, got)
			}
		})
	}
}
