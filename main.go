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

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	argoprojiov1alpha1 "github.com/argoproj-labs/applicationset/api/v1alpha1"
	"github.com/argoproj-labs/applicationset/pkg/controllers"
	"github.com/argoproj-labs/applicationset/pkg/generators"
	"github.com/argoproj-labs/applicationset/pkg/services"
	"github.com/argoproj-labs/applicationset/pkg/utils"

	"github.com/argoproj-labs/applicationset/common"
	argov1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/db"
	argosettings "github.com/argoproj/argo-cd/v2/util/settings"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	appclientset "github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned"
	"github.com/argoproj/pkg/stats"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = argoprojiov1alpha1.AddToScheme(scheme)

	_ = argov1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var probeBindAddr string
	var enableLeaderElection bool
	var namespace string
	var argocdRepoServer string
	var policy string
	var debugLog bool
	var dryRun bool
	var logLevel string

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeBindAddr, "probe-addr", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&namespace, "namespace", "", "Argo CD repo namespace (default: argocd)")
	flag.StringVar(&argocdRepoServer, "argocd-repo-server", "argocd-repo-server:8081", "Argo CD repo server address")
	flag.StringVar(&policy, "policy", "sync", "Modify how application is synced between the generator and the cluster. Default is 'sync' (create & update & delete), options: 'create-only', 'create-update' (no deletion)")
	flag.BoolVar(&debugLog, "debug", false, "Print debug logs. Takes precedence over loglevel")
	flag.StringVar(&logLevel, "loglevel", "info", "Set the logging level. One of: debug|info|warn|error")
	flag.BoolVar(&dryRun, "dry-run", false, "Enable dry run mode")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	policyObj, exists := utils.Policies[policy]
	if !exists {
		setupLog.Info("Policy value can be: sync, create-only, create-update")
		os.Exit(1)
	}

	setLoggingLevel(debugLog, logLevel)

	// If user has not specified a namespace on the CLI, then use the value from NAMESPACE env var
	if len(namespace) == 0 {
		// Determine the namespace we're running in. Normally injected into the pod as an env
		// var via the Kube downward API configured in the Deployment.
		// Developers running the binary locally will need to remember to set the NAMESPACE environment
		// variable, or to use --namespace param
		namespace = os.Getenv("NAMESPACE")
	}

	// If neither the env var, nor the parameter are specified, use the Argo CD default
	if len(namespace) == 0 {
		namespace = "argocd"
	}

	version := common.GetVersion()
	setupLog.Info(fmt.Sprintf("ApplicationSet controller %s using namespace '%s'", version.Version, namespace), "namespace", namespace, "COMMIT_ID", version.GitCommit)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		// Our cache and thus watches and client queries are restricted to the namespace we're running in. This assumes
		// the applicationset controller is in the same namespace as argocd, which should be the same namespace of
		// all cluster Secrets and Applications we interact with.
		NewCache:               cache.MultiNamespacedCacheBuilder([]string{namespace}),
		HealthProbeBindAddress: probeBindAddr,
		Port:                   9443,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "58ac56fa.applicationsets.argoproj.io",
		DryRunClient:           dryRun,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	k8s := kubernetes.NewForConfigOrDie(mgr.GetConfig())
	dynClient := dynamic.NewForConfigOrDie(mgr.GetConfig())
	argoSettingsMgr := argosettings.NewSettingsManager(context.Background(), k8s, namespace)
	appSetConfig := appclientset.NewForConfigOrDie(mgr.GetConfig())

	argoCDDB := db.NewDB(namespace, argoSettingsMgr, k8s)

	// start a webhook server that listens to incoming webhook payloads
	webhookHandler, err := utils.NewWebhookHandler(namespace, argoSettingsMgr, mgr.GetClient())
	if err != nil {
		setupLog.Error(err, "failed to create webhook handler")
		os.Exit(1)
	}
	startWebhookServer(webhookHandler)

	baseGenerators := map[string]generators.Generator{
		"List":                    generators.NewListGenerator(),
		"Clusters":                generators.NewClusterGenerator(mgr.GetClient(), context.Background(), k8s, namespace),
		"Git":                     generators.NewGitGenerator(services.NewArgoCDService(argoCDDB, argocdRepoServer)),
		"SCMProvider":             generators.NewSCMProviderGenerator(mgr.GetClient()),
		"ClusterDecisionResource": generators.NewDuckTypeGenerator(context.Background(), dynClient, k8s, namespace),
	}

	combineGenerators := map[string]generators.Generator{
		"Matrix": generators.NewMatrixGenerator(baseGenerators),
	}

	all, err := generators.CombineMaps(baseGenerators, combineGenerators)
	if err != nil {
		setupLog.Error(err, "generators can't be combined")
		os.Exit(1)
	}

	if err = (&controllers.ApplicationSetReconciler{
		Generators:       all,
		Client:           mgr.GetClient(),
		Log:              ctrl.Log.WithName("controllers").WithName("ApplicationSet"),
		Scheme:           mgr.GetScheme(),
		Recorder:         mgr.GetEventRecorderFor("applicationset-controller"),
		Renderer:         &utils.Render{},
		Policy:           policyObj,
		ArgoAppClientset: appSetConfig,
		KubeClientset:    k8s,
		ArgoDB:           argoCDDB,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ApplicationSet")
		os.Exit(1)
	}

	stats.StartStatsTicker(10 * time.Minute)

	// +kubebuilder:scaffold:builder

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setLoggingLevel(debug bool, logLevel string) {
	// the debug flag takes precedence over the loglevel flag
	if debug {
		log.SetLevel(log.DebugLevel)
	} else {
		level, err := log.ParseLevel(logLevel)
		if err != nil {
			setupLog.Error(err, "unable to parse loglevel", "loglevel", logLevel)
			os.Exit(1)
		}
		log.SetLevel(level)
	}
}

func startWebhookServer(webhookHandler *utils.WebhookHandler) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/webhook", webhookHandler.Handler)
	go func() {
		setupLog.Info("Starting webhook server")
		err := http.ListenAndServe(":7000", mux)
		if err != nil {
			setupLog.Error(err, "failed to start webhook server")
			os.Exit(1)
		}
	}()
}
