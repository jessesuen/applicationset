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

// tests the filters segmentation functions, passing in all the filters, and an unset filter, plus an additional
// branch filter
func TestApplicableFilterMap(t *testing.T) {
	branchFilter := Filter{
		BranchMatch: &regexp.Regexp{},
	}
	repoFilter := Filter{
		RepositoryMatch: &regexp.Regexp{},
	}
	pathExistsFilter := Filter{
		PathsExist: []string{"test"},
	}
	labelMatchFilter := Filter{
		LabelMatch: &regexp.Regexp{},
	}
	unsetFilter := Filter{
		LabelMatch: &regexp.Regexp{},
	}
	additionalBranchFilter := Filter{
		BranchMatch: &regexp.Regexp{},
	}
	bothFilter := Filter{
		RepositoryMatch: &regexp.Regexp{},
		PathsExist:      []string{"test"},
	}
	filters := Filters{&branchFilter, &repoFilter,
		&pathExistsFilter, &labelMatchFilter, &unsetFilter, &additionalBranchFilter, &bothFilter}
	repoFilters := filters.GetRepoFilters()
	branchFilters := filters.GetBranchFilters()

	assert.Len(t, repoFilters, 4)
	assert.Len(t, branchFilters, 4)
}
