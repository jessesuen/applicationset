package common

const (
	// AnnotationGitGeneratorRefresh is an annotation that is added when an ApplicationSet with the git generator is requested to be refreshed by a webhook. The ApplicationSet controller will remove this annotation at the end of reconcilation.
	AnnotationGitGeneratorRefresh = "argocd.argoproj.io/git-gen-refresh"
)
