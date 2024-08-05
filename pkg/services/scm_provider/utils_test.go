package scm_provider

import (
	"context"
	"regexp"
	"testing"

	argoprojiov1alpha1 "github.com/argoproj/applicationset/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func strp(s string) *string {
	return &s
}

func TestFilterRepoMatch(t *testing.T) {
	provider := &MockProvider{
		Repos: []*Repository{
			{
				Repository: "one",
			},
			{
				Repository: "two",
			},
			{
				Repository: "three",
			},
			{
				Repository: "four",
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{
		{
			RepositoryMatch: strp("n|hr"),
		},
	}
	repos, err := ListRepos(context.Background(), provider, filters, "")
	assert.Nil(t, err)
	assert.Len(t, repos, 2)
	assert.Equal(t, "one", repos[0].Repository)
	assert.Equal(t, "three", repos[1].Repository)
}

func TestFilterLabelMatch(t *testing.T) {
	provider := &MockProvider{
		Repos: []*Repository{
			{
				Repository: "one",
				Labels:     []string{"prod-one", "prod-two", "staging"},
			},
			{
				Repository: "two",
				Labels:     []string{"prod-two"},
			},
			{
				Repository: "three",
				Labels:     []string{"staging"},
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{
		{
			LabelMatch: strp("^prod-.*$"),
		},
	}
	repos, err := ListRepos(context.Background(), provider, filters, "")
	assert.Nil(t, err)
	assert.Len(t, repos, 2)
	assert.Equal(t, "one", repos[0].Repository)
	assert.Equal(t, "two", repos[1].Repository)
}

func TestFilterPatchExists(t *testing.T) {
	provider := &MockProvider{
		Repos: []*Repository{
			{
				Repository: "one",
			},
			{
				Repository: "two",
			},
			{
				Repository: "three",
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{
		{
			PathsExist: []string{"two"},
		},
	}
	repos, err := ListRepos(context.Background(), provider, filters, "")
	assert.Nil(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "two", repos[0].Repository)
}

func TestFilterRepoMatchBadRegexp(t *testing.T) {
	provider := &MockProvider{
		Repos: []*Repository{
			{
				Repository: "one",
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{
		{
			RepositoryMatch: strp("("),
		},
	}
	_, err := ListRepos(context.Background(), provider, filters, "")
	assert.NotNil(t, err)
}

func TestFilterLabelMatchBadRegexp(t *testing.T) {
	provider := &MockProvider{
		Repos: []*Repository{
			{
				Repository: "one",
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{
		{
			LabelMatch: strp("("),
		},
	}
	_, err := ListRepos(context.Background(), provider, filters, "")
	assert.NotNil(t, err)
}

func TestFilterBranchMatch(t *testing.T) {
	provider := &MockProvider{
		Repos: []*Repository{
			{
				Repository: "one",
				Branch:     "one",
			},
			{
				Repository: "one",
				Branch:     "two",
			},
			{
				Repository: "two",
				Branch:     "one",
			},
			{
				Repository: "three",
				Branch:     "one",
			},
			{
				Repository: "three",
				Branch:     "two",
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{
		{
			BranchMatch: strp("w"),
		},
	}
	repos, err := ListRepos(context.Background(), provider, filters, "")
	assert.Nil(t, err)
	assert.Len(t, repos, 2)
	assert.Equal(t, "one", repos[0].Repository)
	assert.Equal(t, "two", repos[0].Branch)
	assert.Equal(t, "three", repos[1].Repository)
	assert.Equal(t, "two", repos[1].Branch)
}

func TestMultiFilterAnd(t *testing.T) {
	provider := &MockProvider{
		allPullRequests: false,
		Repos: []*Repository{
			{
				Repository: "one",
				Labels:     []string{"prod-one", "prod-two", "staging"},
			},
			{
				Repository: "two",
				Labels:     []string{"prod-two"},
			},
			{
				Repository: "three",
				Labels:     []string{"staging"},
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{
		{
			RepositoryMatch: strp("w"),
			LabelMatch:      strp("^prod-.*$"),
		},
	}
	repos, err := ListRepos(context.Background(), provider, filters, "")
	assert.Nil(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "two", repos[0].Repository)
}

func TestMultiFilterOr(t *testing.T) {
	provider := &MockProvider{
		Repos: []*Repository{
			{
				Repository: "one",
				Labels:     []string{"prod-one", "prod-two", "staging"},
			},
			{
				Repository: "two",
				Labels:     []string{"prod-two"},
			},
			{
				Repository: "three",
				Labels:     []string{"staging"},
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{
		{
			RepositoryMatch: strp("e"),
		},
		{
			LabelMatch: strp("^prod-.*$"),
		},
	}
	repos, err := ListRepos(context.Background(), provider, filters, "")
	assert.Nil(t, err)
	assert.Len(t, repos, 3)
	assert.Equal(t, "one", repos[0].Repository)
	assert.Equal(t, "two", repos[1].Repository)
	assert.Equal(t, "three", repos[2].Repository)
}

func TestNoFilters(t *testing.T) {
	provider := &MockProvider{
		Repos: []*Repository{
			{
				Repository: "one",
				Labels:     []string{"prod-one", "prod-two", "staging"},
			},
			{
				Repository: "two",
				Labels:     []string{"prod-two"},
			},
			{
				Repository: "three",
				Labels:     []string{"staging"},
			},
		},
	}
	filters := []argoprojiov1alpha1.SCMProviderGeneratorFilter{}
	repos, err := ListRepos(context.Background(), provider, filters, "")
	assert.Nil(t, err)
	assert.Len(t, repos, 3)
	assert.Equal(t, "one", repos[0].Repository)
	assert.Equal(t, "two", repos[1].Repository)
	assert.Equal(t, "three", repos[2].Repository)
}

// tests the getApplicableFilters function, passing in all the filters, and an unset filter, plus an additional
// branch filter
func TestApplicableFilterMap(t *testing.T) {
	branchFilter := Filter{
		BranchMatch: &regexp.Regexp{},
		FilterType:  FilterTypeBranch,
	}
	repoFilter := Filter{
		RepositoryMatch: &regexp.Regexp{},
		FilterType:      FilterTypeRepo,
	}
	pathExistsFilter := Filter{
		PathsExist: []string{"test"},
		FilterType: FilterTypeBranch,
	}
	labelMatchFilter := Filter{
		LabelMatch: &regexp.Regexp{},
		FilterType: FilterTypeRepo,
	}
	unsetFilter := Filter{
		LabelMatch: &regexp.Regexp{},
	}
	additionalBranchFilter := Filter{
		BranchMatch: &regexp.Regexp{},
		FilterType:  FilterTypeBranch,
	}
	filterMap := getApplicableFilters([]*Filter{&branchFilter, &repoFilter,
		&pathExistsFilter, &labelMatchFilter, &unsetFilter, &additionalBranchFilter})

	assert.Len(t, filterMap[FilterTypeRepo], 2)
	assert.Len(t, filterMap[FilterTypeBranch], 3)
}
