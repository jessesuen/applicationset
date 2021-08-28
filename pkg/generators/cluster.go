package generators

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	argoappv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/settings"

	argoappsetv1alpha1 "github.com/argoproj-labs/applicationset/api/v1alpha1"
	"github.com/argoproj-labs/applicationset/pkg/utils"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Generator = (*ClusterGenerator)(nil)

// ClusterGenerator generates Applications for some or all clusters registered with ArgoCD.
type ClusterGenerator struct {
	client.Client
	ctx       context.Context
	clientset kubernetes.Interface
	// namespace is the Argo CD namespace
	namespace       string
	settingsManager *settings.SettingsManager
}

func NewClusterGenerator(c client.Client, ctx context.Context, clientset kubernetes.Interface, namespace string) Generator {

	settingsManager := settings.NewSettingsManager(ctx, clientset, namespace)

	g := &ClusterGenerator{
		Client:          c,
		ctx:             ctx,
		clientset:       clientset,
		namespace:       namespace,
		settingsManager: settingsManager,
	}
	return g
}

func (g *ClusterGenerator) GetRequeueAfter(appSetGenerator *argoappsetv1alpha1.ApplicationSetGenerator) time.Duration {
	return NoRequeueAfter
}

func (g *ClusterGenerator) GetTemplate(appSetGenerator *argoappsetv1alpha1.ApplicationSetGenerator) *argoappsetv1alpha1.ApplicationSetTemplate {
	return &appSetGenerator.Clusters.Template
}

func (g *ClusterGenerator) GenerateParams(
	appSetGenerator *argoappsetv1alpha1.ApplicationSetGenerator, _ *argoappsetv1alpha1.ApplicationSet) ([]map[string]string, error) {

	if appSetGenerator == nil {
		return nil, EmptyAppSetGeneratorError
	}

	if appSetGenerator.Clusters == nil {
		return nil, nil
	}

	paramsSet, err := g.clusterParams(appSetGenerator.Clusters)
	if err != nil {
		return nil, err
	}

	// Finally, enrich all cluster params with the common values.
	for _, params := range paramsSet {
		for key, value := range appSetGenerator.Clusters.Values {
			params[fmt.Sprintf("values.%s", key)] = value
		}
	}

	return paramsSet, nil
}

// clusterParams finds the relevant clusters and generates parameters for each
// one, solely from information held in Argo. Values are ignored at this stage.
func (g *ClusterGenerator) clusterParams(config *argoappsetv1alpha1.ClusterGenerator) ([]map[string]string, error) {
	// If no selector was specified, this will yield all cluster secrets. The
	// local cluster will be included if and only if it has a secret.
	clusterSecrets, err := g.findClusterSecrets(&config.Selector)
	if err != nil {
		return nil, err
	}

	paramsSet := []map[string]string{}

	// If a selector was specified, we just selected the relevant secrets, so
	// generate params for each match.
	if len(config.Selector.MatchExpressions) > 0 ||
		len(config.Selector.MatchLabels) > 0 {
		for _, secret := range clusterSecrets {
			paramsSet = append(paramsSet, paramsForSecret(&secret))
		}
		return paramsSet, nil
	}

	if len(config.Names) > 0 {
		for _, name := range config.Names {
			secret, ok := clusterSecrets[name]
			if !ok {
				log.WithField("cluster", name).Warn("skipping unknown cluster name")
				continue
			}
			paramsSet = append(paramsSet, paramsForSecret(&secret))
		}
		return paramsSet, nil
	}

	// Wildcard selector - all clusters with a secret, plus the local cluster if
	// it does not have a secret.

	// ListClusters provides only the name and server of every cluster, so
	// cross-match with secrets and use those where possible for the additional
	// parameters they provide.
	clusters, err := utils.ListClusters(g.ctx, g.clientset, g.namespace)
	if err != nil {
		return nil, err
	}
	for _, cluster := range clusters.Items {
		if secret, ok := clusterSecrets[cluster.Name]; ok {
			// Remote clusters, and the local cluster if it has a secret.
			paramsSet = append(paramsSet, paramsForSecret(&secret))
		} else {
			// Local cluster, which does not have a secret if we get here.
			paramsSet = append(paramsSet, paramsForCluster(&cluster))
		}
	}
	return paramsSet, nil
}

func paramsForSecret(secret *corev1.Secret) map[string]string {
	params := map[string]string{
		"name":   sanitizeName(string(secret.Data["name"])),
		"server": string(secret.Data["server"]),
	}
	for key, value := range secret.ObjectMeta.Annotations {
		params[fmt.Sprintf("metadata.annotations.%s", key)] = value
	}
	for key, value := range secret.ObjectMeta.Labels {
		params[fmt.Sprintf("metadata.labels.%s", key)] = value
	}
	return params
}

func paramsForCluster(cluster *argoappv1alpha1.Cluster) map[string]string {
	return map[string]string{
		"name":   cluster.Name,
		"server": cluster.Server,
	}
}

const dnsNameMaxLength = 253

var dnsNameInvalidChars = regexp.MustCompile(`[^-a-z0-9.]`)

// sanitizeName returns the provided cluster name, modified if necessary to meet
// the following rules:
//   1. contains no more than 253 characters
//   2. contains only lowercase alphanumeric characters, '-' or '.'
//   3. starts and ends with an alphanumeric character
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = dnsNameInvalidChars.ReplaceAllString(name, "-")
	if len(name) > dnsNameMaxLength {
		name = name[:dnsNameMaxLength]
	}
	return strings.Trim(name, "-.")
}

// findClusterSecrets returns all cluster secret objects matching the provided
// filter, keyed by cluster name. If the filter does not specify
// argocd.argoproj.io/secret-type: cluster, this is added without modifying the
// original object.
func (g *ClusterGenerator) findClusterSecrets(selector *metav1.LabelSelector) (map[string]corev1.Secret, error) {
	secretSelector, err := metav1.LabelSelectorAsSelector(
		metav1.CloneSelectorAndAddLabel(selector, utils.ArgoCDSecretTypeLabel, utils.ArgoCDSecretTypeCluster))
	if err != nil {
		return nil, err
	}

	clusterSecretList := &corev1.SecretList{}
	if err := g.Client.List(g.ctx, clusterSecretList, client.MatchingLabelsSelector{Selector: secretSelector}); err != nil {
		return nil, err
	}
	log.Debug("clusters matching labels", "count", len(clusterSecretList.Items))

	res := map[string]corev1.Secret{}
	for _, cluster := range clusterSecretList.Items {
		clusterName := string(cluster.Data["name"])
		res[clusterName] = cluster
	}

	return res, nil
}
