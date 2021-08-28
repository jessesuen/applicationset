/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Utility struct for a reference to a secret key.
type SecretRef struct {
	SecretName string `json:"secretName"`
	Key        string `json:"key"`
}

// ApplicationSet is a set of Application resources
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=applicationsets,shortName=appset;appsets
type ApplicationSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   ApplicationSetSpec   `json:"spec"`
	Status ApplicationSetStatus `json:"status,omitempty"`
}

// ApplicationSetSpec represents a class of application set state.
type ApplicationSetSpec struct {
	Generators []ApplicationSetGenerator `json:"generators"`
	Template   ApplicationSetTemplate    `json:"template"`
	SyncPolicy *ApplicationSetSyncPolicy `json:"syncPolicy,omitempty"`
}

// ApplicationSetSyncPolicy configures how generated Applications will relate to their
// ApplicationSet.
type ApplicationSetSyncPolicy struct {
	// PreserveResourcesOnDeletion will preserve resources on deletion. If PreserveResourcesOnDeletion is set to true, these Applications will not be deleted.
	PreserveResourcesOnDeletion bool `json:"preserveResourcesOnDeletion,omitempty"`
}

// ApplicationSetTemplate represents argocd ApplicationSpec
type ApplicationSetTemplate struct {
	ApplicationSetTemplateMeta `json:"metadata"`
	Spec                       v1alpha1.ApplicationSpec `json:"spec"`
}

// ApplicationSetTemplateMeta represents the Argo CD application fields that may
// be used for Applications generated from the ApplicationSet (based on metav1.ObjectMeta)
type ApplicationSetTemplateMeta struct {
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Finalizers  []string          `json:"finalizers,omitempty"`
}

// ApplicationSetGenerator include list item info
type ApplicationSetGenerator struct {
	List                    *ListGenerator        `json:"list,omitempty"`
	Clusters                *ClusterGenerator     `json:"clusters,omitempty"`
	Git                     *GitGenerator         `json:"git,omitempty"`
	Matrix                  *MatrixGenerator      `json:"matrix,omitempty"`
	SCMProvider             *SCMProviderGenerator `json:"scmProvider,omitempty"`
	ClusterDecisionResource *DuckTypeGenerator    `json:"clusterDecisionResource,omitempty"`
}

// ApplicationSetBaseGenerator include list item info
// CRD dosn't support recursive types so we need a different type for the matrix generator
// https://github.com/kubernetes-sigs/controller-tools/issues/477
type ApplicationSetBaseGenerator struct {
	List                    *ListGenerator        `json:"list,omitempty"`
	Clusters                *ClusterGenerator     `json:"clusters,omitempty"`
	Git                     *GitGenerator         `json:"git,omitempty"`
	SCMProvider             *SCMProviderGenerator `json:"scmProvider,omitempty"`
	ClusterDecisionResource *DuckTypeGenerator    `json:"clusterDecisionResource,omitempty"`
}

// ListGenerator include items info
type ListGenerator struct {
	Elements []apiextensionsv1.JSON `json:"elements"`
	Template ApplicationSetTemplate `json:"template,omitempty"`
}

// MatrixGenerator include Other generators
type MatrixGenerator struct {
	Generators []ApplicationSetBaseGenerator `json:"generators"`
	Template   ApplicationSetTemplate        `json:"template,omitempty"`
}

// ClusterGenerator defines a generator to match against clusters registered with ArgoCD.
type ClusterGenerator struct {
	// Selector defines a label selector to match against all clusters registered with ArgoCD.
	// Clusters today are stored as Kubernetes Secrets, thus the Secret labels will be used
	// for matching the selector.
	Selector metav1.LabelSelector   `json:"selector,omitempty"`
	Template ApplicationSetTemplate `json:"template,omitempty"`

	// Names defines a set of cluster names to match against all clusters registered with Argo
	// CD. Clusters today are stored as Kubernetes Secrets. The name key is used for matching.
	Names []string `json:"names,omitempty"`

	// Values contains key/value pairs which are passed directly as parameters to the template
	Values map[string]string `json:"values,omitempty"`
}

// DuckType defines a generator to match against clusters registered with ArgoCD.
type DuckTypeGenerator struct {
	// ConfigMapRef is a ConfigMap with the duck type definitions needed to retreive the data
	//              this includes apiVersion(group/version), kind, matchKey and validation settings
	// Name is the resource name of the kind, group and version, defined in the ConfigMapRef
	// RequeueAfterSeconds is how long before the duckType will be rechecked for a change
	ConfigMapRef        string               `json:"configMapRef"`
	Name                string               `json:"name,omitempty"`
	RequeueAfterSeconds *int64               `json:"requeueAfterSeconds,omitempty"`
	LabelSelector       metav1.LabelSelector `json:"labelSelector,omitempty"`

	Template ApplicationSetTemplate `json:"template,omitempty"`
	// Values contains key/value pairs which are passed directly as parameters to the template
	Values map[string]string `json:"values,omitempty"`
}

type GitGenerator struct {
	RepoURL             string                      `json:"repoURL"`
	Directories         []GitDirectoryGeneratorItem `json:"directories,omitempty"`
	Files               []GitFileGeneratorItem      `json:"files,omitempty"`
	Revision            string                      `json:"revision"`
	RequeueAfterSeconds *int64                      `json:"requeueAfterSeconds,omitempty"`
	Template            ApplicationSetTemplate      `json:"template,omitempty"`
}

type GitDirectoryGeneratorItem struct {
	Path    string `json:"path"`
	Exclude bool   `json:"exclude,omitempty"`
}

type GitFileGeneratorItem struct {
	Path string `json:"path"`
}

// SCMProviderGenerator defines a generator that scrapes a SCMaaS API to find candidate repos.
type SCMProviderGenerator struct {
	// Which provider to use and config for it.
	Github *SCMProviderGeneratorGithub `json:"github,omitempty"`
	Gitlab *SCMProviderGeneratorGitlab `json:"gitlab,omitempty"`
	// Filters for which repos should be considered.
	Filters []SCMProviderGeneratorFilter `json:"filters,omitempty"`
	// Which protocol to use for the SCM URL. Default is provider-specific but ssh if possible. Not all providers
	// necessarily support all protocols.
	CloneProtocol string `json:"cloneProtocol,omitempty"`
	// Standard parameters.
	RequeueAfterSeconds *int64                 `json:"requeueAfterSeconds,omitempty"`
	Template            ApplicationSetTemplate `json:"template,omitempty"`
}

// SCMProviderGeneratorGithub defines a connection info specific to GitHub.
type SCMProviderGeneratorGithub struct {
	// GitHub org to scan. Required.
	Organization string `json:"organization"`
	// The GitHub API URL to talk to. If blank, use https://api.github.com/.
	API string `json:"api,omitempty"`
	// Authentication token reference.
	TokenRef *SecretRef `json:"tokenRef,omitempty"`
	// Scan all branches instead of just the default branch.
	AllBranches bool `json:"allBranches,omitempty"`
}

// SCMProviderGeneratorGitlab defines a connection info specific to Gitlab.
type SCMProviderGeneratorGitlab struct {
	// Gitlab group to scan. Required.  You can use either the project id (recommended) or the full namespaced path.
	Group string `json:"group"`
	// Recurse through subgroups (true) or scan only the base group (false).  Defaults to "false"
	IncludeSubgroups bool `json:"includeSubgroups,omitempty"`
	// The Gitlab API URL to talk to.
	API string `json:"api,omitempty"`
	// Authentication token reference.
	TokenRef *SecretRef `json:"tokenRef,omitempty"`
	// Scan all branches instead of just the default branch.
	AllBranches bool `json:"allBranches,omitempty"`
}

// SCMProviderGeneratorFilter is a single repository filter.
// If multiple filter types are set on a single struct, they will be AND'd together. All filters must
// pass for a repo to be included.
type SCMProviderGeneratorFilter struct {
	// A regex for repo names.
	RepositoryMatch *string `json:"repositoryMatch,omitempty"`
	// An array of paths, all of which must exist.
	PathsExist []string `json:"pathsExist,omitempty"`
	// A regex which must match at least one label.
	LabelMatch *string `json:"labelMatch,omitempty"`
	// A regex which must match the branch name.
	BranchMatch *string `json:"branchMatch,omitempty"`
}

// ApplicationSetStatus defines the observed state of ApplicationSet
type ApplicationSetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// ApplicationSetList contains a list of ApplicationSet
// +kubebuilder:object:root=true
type ApplicationSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApplicationSet{}, &ApplicationSetList{})
}
