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
}

type SCMProviderService interface {
	ListRepos(context.Context, string) ([]*Repository, error)
	RepoHasPath(context.Context, *Repository, string) (bool, error)
}

// A compiled version of SCMProviderGeneratorFilter for performance.
type Filter struct {
	RepositoryMatch *regexp.Regexp
	PathsExist      []string
	LabelMatch      *regexp.Regexp
	BranchMatch     *regexp.Regexp
}
