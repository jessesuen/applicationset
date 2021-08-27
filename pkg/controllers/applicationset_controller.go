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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/argoproj-labs/applicationset/common"
	"github.com/argoproj-labs/applicationset/pkg/generators"
	"github.com/argoproj-labs/applicationset/pkg/utils"
	argov1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/util/db"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	argoprojiov1alpha1 "github.com/argoproj-labs/applicationset/api/v1alpha1"
	appclientset "github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned"
	argoutil "github.com/argoproj/argo-cd/v2/util/argo"

	apierr "k8s.io/apimachinery/pkg/api/errors"
)

const (
	// Rather than importing the whole argocd-notifications controller, just copying the const here
	//   https://github.com/argoproj-labs/argocd-notifications/blob/33d345fa838829bb50fca5c08523aba380d2c12b/pkg/controller/subscriptions.go#L12
	//   https://github.com/argoproj-labs/argocd-notifications/blob/33d345fa838829bb50fca5c08523aba380d2c12b/pkg/controller/state.go#L17
	NotifiedAnnotationKey             = "notified.notifications.argoproj.io"
	ReconcileRequeueOnValidationError = time.Minute * 3
)

// ApplicationSetReconciler reconciles a ApplicationSet object
type ApplicationSetReconciler struct {
	client.Client
	Log              logr.Logger
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
	Generators       map[string]generators.Generator
	ArgoDB           db.ArgoDB
	ArgoAppClientset appclientset.Interface
	KubeClientset    kubernetes.Interface
	utils.Policy
	utils.Renderer
}

// +kubebuilder:rbac:groups=argoproj.io,resources=applicationsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=applicationsets/status,verbs=get;update;patch

func (r *ApplicationSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("applicationset", req.NamespacedName)
	_ = log.WithField("applicationset", req.NamespacedName)

	var applicationSetInfo argoprojiov1alpha1.ApplicationSet
	if err := r.Get(ctx, req.NamespacedName, &applicationSetInfo); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.WithField("request", req).WithError(err).Infof("unable to get ApplicationSet")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Do not attempt to further reconcile the ApplicationSet if it is being deleted.
	if applicationSetInfo.ObjectMeta.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// Log a warning if there are unrecognized generators
	utils.CheckInvalidGenerators(&applicationSetInfo)

	// desiredApplications is the main list of all expected Applications from all generators in this appset.
	desiredApplications, err := r.generateApplications(applicationSetInfo)
	if err != nil {
		return ctrl.Result{}, err
	}

	if validateError := r.validateGeneratedApplications(ctx, desiredApplications, applicationSetInfo, req.Namespace); validateError != nil {
		// While some generators may return an error that requires user intervention,
		// other generators reference external resources that may change to cause
		// the error to no longer occur. We thus log the error and requeue
		// with a timeout to give this another shot at a later time.
		//
		// Changes to watched resources will cause this to be reconciled sooner than
		// the RequeueAfter time.
		log.Errorf("error occurred during application generation: %s", validateError.Error())
		return ctrl.Result{RequeueAfter: ReconcileRequeueOnValidationError}, nil
	}

	if r.Policy.Update() {
		err = r.createOrUpdateInCluster(ctx, applicationSetInfo, desiredApplications)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		err = r.createInCluster(ctx, applicationSetInfo, desiredApplications)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if r.Policy.Delete() {
		err = r.deleteInCluster(ctx, applicationSetInfo, desiredApplications)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if applicationSetInfo.RefreshRequired() {
		delete(applicationSetInfo.Annotations, common.AnnotationGitGeneratorRefresh)
		err := r.Client.Update(ctx, &applicationSetInfo)
		if err != nil {
			log.Warnf("error occurred while updating ApplicationSet: %v", err)
			return ctrl.Result{}, err
		}
	}

	requeueAfter := r.getMinRequeueAfter(&applicationSetInfo)
	log.WithField("requeueAfter", requeueAfter).Info("end reconcile")

	return ctrl.Result{
		RequeueAfter: requeueAfter,
	}, nil
}

// validateGeneratedApplications uses the Argo CD validation functions to verify the correctness of the
// generated applications.
func (r *ApplicationSetReconciler) validateGeneratedApplications(ctx context.Context, desiredApplications []argov1alpha1.Application, applicationSetInfo argoprojiov1alpha1.ApplicationSet, namespace string) (err error) {

	if hasDuplicates, name := hasDuplicateNames(desiredApplications); hasDuplicates {
		return fmt.Errorf("ApplicationSet %s contains applications with duplicate name: %s", applicationSetInfo.Name, name)
	}

	for _, app := range desiredApplications {

		proj, err := r.ArgoAppClientset.ArgoprojV1alpha1().AppProjects(namespace).Get(ctx, app.Spec.GetProject(), metav1.GetOptions{})
		if err != nil {
			if apierr.IsNotFound(err) {
				return fmt.Errorf("application references project %s which does not exist", app.Spec.Project)
			}
			return err
		}

		if err := utils.ValidateDestination(ctx, &app.Spec.Destination, r.KubeClientset, namespace); err != nil {
			return fmt.Errorf("application destination spec is invalid: %s", err.Error())
		}

		conditions, err := argoutil.ValidatePermissions(ctx, &app.Spec, proj, r.ArgoDB)
		if err != nil {
			return err
		}
		if len(conditions) > 0 {
			return fmt.Errorf("application spec is invalid: %s", argoutil.FormatAppConditions(conditions))
		}

	}

	return nil
}

func hasDuplicateNames(applications []argov1alpha1.Application) (bool, string) {
	nameSet := map[string]struct{}{}
	for _, app := range applications {
		if _, present := nameSet[app.Name]; present {
			return true, app.Name
		}
		nameSet[app.Name] = struct{}{}
	}
	return false, ""
}

func (r *ApplicationSetReconciler) getMinRequeueAfter(applicationSetInfo *argoprojiov1alpha1.ApplicationSet) time.Duration {
	var res time.Duration
	for _, requestedGenerator := range applicationSetInfo.Spec.Generators {

		relevantGenerators := generators.GetRelevantGenerators(&requestedGenerator, r.Generators)

		for _, g := range relevantGenerators {
			t := g.GetRequeueAfter(&requestedGenerator)

			if res == 0 {
				res = t
			} else if t != 0 && t < res {
				res = t
			}
		}
	}

	return res
}

func getTempApplication(applicationSetTemplate argoprojiov1alpha1.ApplicationSetTemplate) *argov1alpha1.Application {
	var tmplApplication argov1alpha1.Application
	tmplApplication.Annotations = applicationSetTemplate.Annotations
	tmplApplication.Labels = applicationSetTemplate.Labels
	tmplApplication.Namespace = applicationSetTemplate.Namespace
	tmplApplication.Name = applicationSetTemplate.Name
	tmplApplication.Spec = applicationSetTemplate.Spec
	tmplApplication.Finalizers = applicationSetTemplate.Finalizers

	return &tmplApplication
}

func (r *ApplicationSetReconciler) generateApplications(applicationSetInfo argoprojiov1alpha1.ApplicationSet) ([]argov1alpha1.Application, error) {
	var res []argov1alpha1.Application

	var firstError error
	for _, requestedGenerator := range applicationSetInfo.Spec.Generators {
		t, err := generators.Transform(requestedGenerator, r.Generators, applicationSetInfo.Spec.Template, &applicationSetInfo)
		if err != nil {
			log.WithError(err).WithField("generator", requestedGenerator).
				Error("error generating application from params")
			if firstError == nil {
				firstError = err
			}
			continue
		}

		for _, a := range t {
			tmplApplication := getTempApplication(a.Template)

			for _, p := range a.Params {
				app, err := r.Renderer.RenderTemplateParams(tmplApplication, applicationSetInfo.Spec.SyncPolicy, p)
				if err != nil {
					log.WithError(err).WithField("params", a.Params).WithField("generator", requestedGenerator).
						Error("error generating application from params")
					if firstError == nil {
						firstError = err
					}
					continue
				}
				res = append(res, *app)
			}
		}

		log.WithField("generator", requestedGenerator).Infof("generated %d applications", len(res))
		log.WithField("generator", requestedGenerator).Debugf("apps from generator: %+v", res)

	}

	return res, firstError
}

func (r *ApplicationSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &argov1alpha1.Application{}, ".metadata.controller", func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		app := rawObj.(*argov1alpha1.Application)
		owner := metav1.GetControllerOf(app)
		if owner == nil {
			return nil
		}
		// ...make sure it's a application set...
		if owner.APIVersion != argoprojiov1alpha1.GroupVersion.String() || owner.Kind != "ApplicationSet" {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&argoprojiov1alpha1.ApplicationSet{}).
		Owns(&argov1alpha1.Application{}).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			&clusterSecretEventHandler{
				Client: mgr.GetClient(),
				Log:    log.WithField("type", "createSecretEventHandler"),
			}).
		// TODO: also watch Applications and respond on changes if we own them.
		Complete(r)
}

// createOrUpdateInCluster will create / update application resources in the cluster.
// - For new applications, it will call create
// - For existing application, it will call update
// The function also adds owner reference to all applications, and uses it to delete them.
func (r *ApplicationSetReconciler) createOrUpdateInCluster(ctx context.Context, applicationSet argoprojiov1alpha1.ApplicationSet, desiredApplications []argov1alpha1.Application) error {

	var firstError error
	// Creates or updates the application in appList
	for _, generatedApp := range desiredApplications {

		appLog := log.WithFields(log.Fields{"app": generatedApp.Name, "appSet": applicationSet.Name})
		generatedApp.Namespace = applicationSet.Namespace

		found := &argov1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      generatedApp.Name,
				Namespace: generatedApp.Namespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "argoproj.io/v1alpha1",
			},
		}

		action, err := utils.CreateOrUpdate(ctx, r.Client, found, func() error {
			// Copy only the Application/ObjectMeta fields that are significant, from the generatedApp
			found.Spec = generatedApp.Spec

			// Preserve argo cd notifications state (https://github.com/argoproj-labs/applicationset/issues/180)
			if state, exists := found.ObjectMeta.Annotations[NotifiedAnnotationKey]; exists {
				if generatedApp.Annotations == nil {
					generatedApp.Annotations = map[string]string{}
				}
				generatedApp.Annotations[NotifiedAnnotationKey] = state
			}
			found.ObjectMeta.Annotations = generatedApp.Annotations

			found.ObjectMeta.Finalizers = generatedApp.Finalizers
			found.ObjectMeta.Labels = generatedApp.Labels
			return controllerutil.SetControllerReference(&applicationSet, found, r.Scheme)
		})

		if err != nil {
			appLog.WithError(err).WithField("action", action).Errorf("failed to %s Application", action)
			if firstError == nil {
				firstError = err
			}
			continue
		}

		r.Recorder.Eventf(&applicationSet, corev1.EventTypeNormal, fmt.Sprint(action), "%s Application %q", action, generatedApp.Name)
		appLog.Logf(log.InfoLevel, "%s Application", action)
	}
	return firstError
}

// createInCluster will filter from the desiredApplications only the application that needs to be created
// Then it will call createOrUpdateInCluster to do the actual create
func (r *ApplicationSetReconciler) createInCluster(ctx context.Context, applicationSet argoprojiov1alpha1.ApplicationSet, desiredApplications []argov1alpha1.Application) error {

	var createApps []argov1alpha1.Application
	current, err := r.getCurrentApplications(ctx, applicationSet)
	if err != nil {
		return err
	}

	m := make(map[string]bool) // Will holds the app names that are current in the cluster

	for _, app := range current {
		m[app.Name] = true
	}

	// filter applications that are not in m[string]bool (new to the cluster)
	for _, app := range desiredApplications {
		_, exists := m[app.Name]

		if !exists {
			createApps = append(createApps, app)
		}
	}

	return r.createOrUpdateInCluster(ctx, applicationSet, createApps)
}

func (r *ApplicationSetReconciler) getCurrentApplications(_ context.Context, applicationSet argoprojiov1alpha1.ApplicationSet) ([]argov1alpha1.Application, error) {
	// TODO: Should this use the context param?
	var current argov1alpha1.ApplicationList
	err := r.Client.List(context.Background(), &current, client.MatchingFields{".metadata.controller": applicationSet.Name})

	if err != nil {
		return nil, err
	}

	return current.Items, nil
}

// deleteInCluster will delete Applications that are currently on the cluster, but not in appList.
// The function must be called after all generators had been called and generated applications
func (r *ApplicationSetReconciler) deleteInCluster(ctx context.Context, applicationSet argoprojiov1alpha1.ApplicationSet, desiredApplications []argov1alpha1.Application) error {

	clusterList, err := utils.ListClusters(ctx, r.KubeClientset, applicationSet.Namespace)
	if err != nil {
		return err
	}

	// Save current applications to be able to delete the ones that are not in appList
	current, err := r.getCurrentApplications(ctx, applicationSet)
	if err != nil {
		return err
	}

	m := make(map[string]bool) // Will holds the app names in appList for the deletion process

	for _, app := range desiredApplications {
		m[app.Name] = true
	}

	// Delete apps that are not in m[string]bool
	var firstError error
	for _, app := range current {
		appLog := log.WithFields(log.Fields{"app": app.Name, "appSet": applicationSet.Name})
		_, exists := m[app.Name]

		if !exists {

			// Removes the Argo CD resources finalizer if the application contains an invalid target (eg missing cluster)
			err := r.removeFinalizerOnInvalidDestination(ctx, applicationSet, &app, clusterList, appLog)
			if err != nil {
				appLog.WithError(err).Error("failed to update Application")
				if firstError != nil {
					firstError = err
				}
				continue
			}

			err = r.Client.Delete(ctx, &app)
			if err != nil {
				appLog.WithError(err).Error("failed to delete Application")
				if firstError != nil {
					firstError = err
				}
				continue
			}
			r.Recorder.Eventf(&applicationSet, corev1.EventTypeNormal, "Deleted", "Deleted Application %q", app.Name)
			appLog.Log(log.InfoLevel, "Deleted application")
		}
	}
	return firstError
}

// removeFinalizerOnInvalidDestination removes the Argo CD resources finalizer if the application contains an invalid target (eg missing cluster)
func (r *ApplicationSetReconciler) removeFinalizerOnInvalidDestination(ctx context.Context, applicationSet argoprojiov1alpha1.ApplicationSet, app *argov1alpha1.Application, clusterList *argov1alpha1.ClusterList, appLog *log.Entry) error {

	// Only check if the finalizers need to be removed IF there are finalizers to remove
	if len(app.Finalizers) == 0 {
		return nil
	}

	var validDestination bool

	// Detect if the destination is invalid (name doesn't correspond to a matching cluster)
	if err := utils.ValidateDestination(ctx, &app.Spec.Destination, r.KubeClientset, applicationSet.Namespace); err != nil {
		appLog.Warnf("The destination cluster for %s couldn't be found: %v", app.Name, err)
		validDestination = false
	} else {

		// Detect if the destination's server field does not match an existing cluster

		matchingCluster := false
		for _, cluster := range clusterList.Items {

			// Server fields must match. Note that ValidateDestination ensures that the server field is set, if applicable.
			if app.Spec.Destination.Server != cluster.Server {
				continue
			}

			// The name must match, if it is not empty
			if app.Spec.Destination.Name != "" && cluster.Name != app.Spec.Destination.Name {
				continue
			}

			matchingCluster = true
			break
		}

		if !matchingCluster {
			appLog.Warnf("A match for the destination cluster for %s, by server url, couldn't be found.", app.Name)
		}

		validDestination = matchingCluster
	}
	// If the destination is invalid (for example the cluster is no longer defined), then remove
	// the application finalizers to avoid triggering Argo CD bug #5817
	if !validDestination {

		// Filter out the Argo CD finalizer from the finalizer list
		var newFinalizers []string
		for _, existingFinalizer := range app.Finalizers {
			if existingFinalizer != argov1alpha1.ResourcesFinalizerName { // only remove this one
				newFinalizers = append(newFinalizers, existingFinalizer)
			}
		}

		// If the finalizer length changed (due to filtering out an Argo finalizer), update the finalizer list on the app
		if len(newFinalizers) != len(app.Finalizers) {
			app.Finalizers = newFinalizers

			r.Recorder.Eventf(&applicationSet, corev1.EventTypeNormal, "Updated", "Updated Application %q finalizer before deletion, because application has an invalid destination", app.Name)
			appLog.Log(log.InfoLevel, "Updating application finalizer before deletion, because application has an invalid destination")

			err := r.Client.Update(ctx, app, &client.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

var _ handler.EventHandler = &clusterSecretEventHandler{}
