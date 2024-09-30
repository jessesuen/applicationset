package scm_provider

import (
	"context"
	"regexp"
)

// An abstract repository from an API provider.
type Repository struct {
	Organization string
	Repository   string
	URL          string
	Branch       string
	SHA          string
	Labels       []string
	RepositoryId interface{}
}

type SCMProviderService interface {
	ListRepos(context.Context, string) ([]*Repository, error)
	RepoHasPath(context.Context, *Repository, string) (bool, error)
	GetBranches(context.Context, *Repository) ([]*Repository, error)
}

// A compiled version of SCMProviderGeneratorFilter for performance.
type Filter struct {
	RepositoryMatch *regexp.Regexp
	PathsExist      []string
	LabelMatch      *regexp.Regexp
	BranchMatch     *regexp.Regexp
}

// HasRepoFilter returns true if a Filter includes a filter which can exclude a repo based on knowledge about the repo
// only (no need for knowledge of any branches). A Filter with a "repo filter" may also include branch-specific filters.
func (f *Filter) HasRepoFilter() bool {
	return f.RepositoryMatch != nil || f.LabelMatch != nil
}

// HasBranchFilter returns true if a Filter includes a filter which can exclude a branch.
func (f *Filter) HasBranchFilter() bool {
	return f.BranchMatch != nil || f.PathsExist != nil
}

type Filters []*Filter

func (f Filters) GetRepoFilters() Filters {
	var repoFilters Filters
	for _, filter := range f {
		if filter.HasRepoFilter() {
			repoFilters = append(repoFilters, filter)
		}
	}
	return repoFilters
}

func (f Filters) GetBranchFilters() Filters {
	var branchFilters Filters
	for _, filter := range f {
		if filter.HasBranchFilter() {
			branchFilters = append(branchFilters, filter)
		}
	}
	return branchFilters
}

// A convenience type for indicating where to apply a filter
type FilterType int64

// The enum of filter types
const (
	FilterTypeUndefined FilterType = iota
	FilterTypeBranch
	FilterTypeRepo
)
