package applicationsets

import (
	"testing"

	"github.com/argoproj-labs/applicationset/api/v1alpha1"
	. "github.com/argoproj-labs/applicationset/test/e2e/fixture/applicationsets"
	"github.com/argoproj-labs/applicationset/test/e2e/fixture/applicationsets/utils"
	argov1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSimpleClusterGenerator(t *testing.T) {

	expectedApp := argov1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster1-guestbook",
			Namespace:  utils.ArgoCDNamespace,
			Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
		},
		Spec: argov1alpha1.ApplicationSpec{
			Project: "default",
			Source: argov1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
				TargetRevision: "HEAD",
				Path:           "guestbook",
			},
			Destination: argov1alpha1.ApplicationDestination{
				Name:      "cluster1",
				Namespace: "guestbook",
			},
		},
	}

	var expectedAppNewNamespace *argov1alpha1.Application
	var expectedAppNewMetadata *argov1alpha1.Application

	Given(t).
		// Create a ClusterGenerator-based ApplicationSet
		When().
		CreateClusterSecret("my-secret", "cluster1", "https://kubernetes.default.svc").
		Create(v1alpha1.ApplicationSet{ObjectMeta: metav1.ObjectMeta{
			Name: "simple-cluster-generator",
		},
			Spec: v1alpha1.ApplicationSetSpec{
				Template: v1alpha1.ApplicationSetTemplate{
					ApplicationSetTemplateMeta: v1alpha1.ApplicationSetTemplateMeta{Name: "{{name}}-guestbook"},
					Spec: argov1alpha1.ApplicationSpec{
						Project: "default",
						Source: argov1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
							TargetRevision: "HEAD",
							Path:           "guestbook",
						},
						Destination: argov1alpha1.ApplicationDestination{
							Name: "{{name}}",
							// Server:    "{{server}}",
							Namespace: "guestbook",
						},
					},
				},
				Generators: []v1alpha1.ApplicationSetGenerator{
					{
						Clusters: &v1alpha1.ClusterGenerator{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"argocd.argoproj.io/secret-type": "cluster",
								},
							},
						},
					},
				},
			},
		}).Then().Expect(ApplicationsExist([]argov1alpha1.Application{expectedApp})).

		// Update the ApplicationSet template namespace, and verify it updates the Applications
		When().
		And(func() {
			expectedAppNewNamespace = expectedApp.DeepCopy()
			expectedAppNewNamespace.Spec.Destination.Namespace = "guestbook2"
		}).
		Update(func(appset *v1alpha1.ApplicationSet) {
			appset.Spec.Template.Spec.Destination.Namespace = "guestbook2"
		}).Then().Expect(ApplicationsExist([]argov1alpha1.Application{*expectedAppNewNamespace})).

		// Update the metadata fields in the appset template, and make sure it propagates to the apps
		When().
		And(func() {
			expectedAppNewMetadata = expectedAppNewNamespace.DeepCopy()
			expectedAppNewMetadata.ObjectMeta.Annotations = map[string]string{"annotation-key": "annotation-value"}
			expectedAppNewMetadata.ObjectMeta.Labels = map[string]string{"label-key": "label-value"}
		}).
		Update(func(appset *v1alpha1.ApplicationSet) {
			appset.Spec.Template.Annotations = map[string]string{"annotation-key": "annotation-value"}
			appset.Spec.Template.Labels = map[string]string{"label-key": "label-value"}
		}).Then().Expect(ApplicationsExist([]argov1alpha1.Application{*expectedAppNewMetadata})).

		// Delete the ApplicationSet, and verify it deletes the Applications
		When().
		Delete().Then().Expect(ApplicationsDoNotExist([]argov1alpha1.Application{*expectedAppNewNamespace}))
}

func TestClusterGeneratorWithLocalCluster(t *testing.T) {

	expectedAppTemplate := argov1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "in-cluster-guestbook",
			Namespace:  utils.ArgoCDNamespace,
			Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
		},
		Spec: argov1alpha1.ApplicationSpec{
			Project: "default",
			Source: argov1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
				TargetRevision: "HEAD",
				Path:           "guestbook",
			},
			// Destination comes from appDestination below
		},
	}

	tests := []struct {
		name              string
		appsetDestination argov1alpha1.ApplicationDestination
		appDestination    argov1alpha1.ApplicationDestination
	}{
		{
			name: "specify local cluster by server field",
			appDestination: argov1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: "guestbook",
			},
			appsetDestination: argov1alpha1.ApplicationDestination{
				Server:    "{{server}}",
				Namespace: "guestbook",
			},
		},
		{
			name: "specify local cluster by name field",
			appDestination: argov1alpha1.ApplicationDestination{
				Name:      "in-cluster",
				Namespace: "guestbook",
			},
			appsetDestination: argov1alpha1.ApplicationDestination{
				Name:      "{{name}}",
				Namespace: "guestbook",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var expectedAppNewNamespace *argov1alpha1.Application
			var expectedAppNewMetadata *argov1alpha1.Application

			// Create the expected application from the template, and copy in the destination from the test case
			expectedApp := *expectedAppTemplate.DeepCopy()
			expectedApp.Spec.Destination = test.appDestination

			Given(t).
				// Create a ClusterGenerator-based ApplicationSet
				When().
				Create(v1alpha1.ApplicationSet{ObjectMeta: metav1.ObjectMeta{
					Name: "in-cluster-generator",
				},
					Spec: v1alpha1.ApplicationSetSpec{
						Template: v1alpha1.ApplicationSetTemplate{
							ApplicationSetTemplateMeta: v1alpha1.ApplicationSetTemplateMeta{Name: "{{name}}-guestbook"},
							Spec: argov1alpha1.ApplicationSpec{
								Project: "default",
								Source: argov1alpha1.ApplicationSource{
									RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
									TargetRevision: "HEAD",
									Path:           "guestbook",
								},
								Destination: test.appsetDestination,
							},
						},
						Generators: []v1alpha1.ApplicationSetGenerator{
							{
								Clusters: &v1alpha1.ClusterGenerator{},
							},
						},
					},
				}).Then().Expect(ApplicationsExist([]argov1alpha1.Application{expectedApp})).

				// Update the ApplicationSet template namespace, and verify it updates the Applications
				When().
				And(func() {
					expectedAppNewNamespace = expectedApp.DeepCopy()
					expectedAppNewNamespace.Spec.Destination.Namespace = "guestbook2"
				}).
				Update(func(appset *v1alpha1.ApplicationSet) {
					appset.Spec.Template.Spec.Destination.Namespace = "guestbook2"
				}).Then().Expect(ApplicationsExist([]argov1alpha1.Application{*expectedAppNewNamespace})).

				// Update the metadata fields in the appset template, and make sure it propagates to the apps
				When().
				And(func() {
					expectedAppNewMetadata = expectedAppNewNamespace.DeepCopy()
					expectedAppNewMetadata.ObjectMeta.Annotations = map[string]string{"annotation-key": "annotation-value"}
					expectedAppNewMetadata.ObjectMeta.Labels = map[string]string{"label-key": "label-value"}
				}).
				Update(func(appset *v1alpha1.ApplicationSet) {
					appset.Spec.Template.Annotations = map[string]string{"annotation-key": "annotation-value"}
					appset.Spec.Template.Labels = map[string]string{"label-key": "label-value"}
				}).Then().Expect(ApplicationsExist([]argov1alpha1.Application{*expectedAppNewMetadata})).

				// Delete the ApplicationSet, and verify it deletes the Applications
				When().
				Delete().Then().Expect(ApplicationsDoNotExist([]argov1alpha1.Application{*expectedAppNewNamespace}))
		})
	}
}

func TestSimpleClusterGeneratorAddingCluster(t *testing.T) {

	expectedAppTemplate := argov1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "{{name}}-guestbook",
			Namespace:  utils.ArgoCDNamespace,
			Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
		},
		Spec: argov1alpha1.ApplicationSpec{
			Project: "default",
			Source: argov1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
				TargetRevision: "HEAD",
				Path:           "guestbook",
			},
			Destination: argov1alpha1.ApplicationDestination{
				Name:      "{{name}}",
				Namespace: "guestbook",
			},
		},
	}

	expectedAppCluster1 := *expectedAppTemplate.DeepCopy()
	expectedAppCluster1.Spec.Destination.Name = "cluster1"
	expectedAppCluster1.ObjectMeta.Name = "cluster1-guestbook"

	expectedAppCluster2 := *expectedAppTemplate.DeepCopy()
	expectedAppCluster2.Spec.Destination.Name = "cluster2"
	expectedAppCluster2.ObjectMeta.Name = "cluster2-guestbook"

	Given(t).
		// Create a ClusterGenerator-based ApplicationSet
		When().
		CreateClusterSecret("my-secret", "cluster1", "https://kubernetes.default.svc").
		Create(v1alpha1.ApplicationSet{ObjectMeta: metav1.ObjectMeta{
			Name: "simple-cluster-generator",
		},
			Spec: v1alpha1.ApplicationSetSpec{
				Template: v1alpha1.ApplicationSetTemplate{
					ApplicationSetTemplateMeta: v1alpha1.ApplicationSetTemplateMeta{Name: "{{name}}-guestbook"},
					Spec: argov1alpha1.ApplicationSpec{
						Project: "default",
						Source: argov1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
							TargetRevision: "HEAD",
							Path:           "guestbook",
						},
						Destination: argov1alpha1.ApplicationDestination{
							Name: "{{name}}",
							// Server:    "{{server}}",
							Namespace: "guestbook",
						},
					},
				},
				Generators: []v1alpha1.ApplicationSetGenerator{
					{
						Clusters: &v1alpha1.ClusterGenerator{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"argocd.argoproj.io/secret-type": "cluster",
								},
							},
						},
					},
				},
			},
		}).Then().Expect(ApplicationsExist([]argov1alpha1.Application{expectedAppCluster1})).

		// Update the ApplicationSet template namespace, and verify it updates the Applications
		When().
		CreateClusterSecret("my-secret2", "cluster2", "https://kubernetes.default.svc").
		Then().Expect(ApplicationsExist([]argov1alpha1.Application{expectedAppCluster1, expectedAppCluster2})).

		// Delete the ApplicationSet, and verify it deletes the Applications
		When().
		Delete().Then().Expect(ApplicationsDoNotExist([]argov1alpha1.Application{expectedAppCluster1, expectedAppCluster2}))
}

func TestSimpleClusterGeneratorDeletingCluster(t *testing.T) {

	expectedAppTemplate := argov1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "{{name}}-guestbook",
			Namespace:  utils.ArgoCDNamespace,
			Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
		},
		Spec: argov1alpha1.ApplicationSpec{
			Project: "default",
			Source: argov1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
				TargetRevision: "HEAD",
				Path:           "guestbook",
			},
			Destination: argov1alpha1.ApplicationDestination{
				Name:      "{{name}}",
				Namespace: "guestbook",
			},
		},
	}

	expectedAppCluster1 := *expectedAppTemplate.DeepCopy()
	expectedAppCluster1.Spec.Destination.Name = "cluster1"
	expectedAppCluster1.ObjectMeta.Name = "cluster1-guestbook"

	expectedAppCluster2 := *expectedAppTemplate.DeepCopy()
	expectedAppCluster2.Spec.Destination.Name = "cluster2"
	expectedAppCluster2.ObjectMeta.Name = "cluster2-guestbook"

	Given(t).
		// Create a ClusterGenerator-based ApplicationSet
		When().
		CreateClusterSecret("my-secret", "cluster1", "https://kubernetes.default.svc").
		CreateClusterSecret("my-secret2", "cluster2", "https://kubernetes.default.svc").
		Create(v1alpha1.ApplicationSet{ObjectMeta: metav1.ObjectMeta{
			Name: "simple-cluster-generator",
		},
			Spec: v1alpha1.ApplicationSetSpec{
				Template: v1alpha1.ApplicationSetTemplate{
					ApplicationSetTemplateMeta: v1alpha1.ApplicationSetTemplateMeta{Name: "{{name}}-guestbook"},
					Spec: argov1alpha1.ApplicationSpec{
						Project: "default",
						Source: argov1alpha1.ApplicationSource{
							RepoURL:        "https://github.com/argoproj/argocd-example-apps.git",
							TargetRevision: "HEAD",
							Path:           "guestbook",
						},
						Destination: argov1alpha1.ApplicationDestination{
							Name: "{{name}}",
							// Server:    "{{server}}",
							Namespace: "guestbook",
						},
					},
				},
				Generators: []v1alpha1.ApplicationSetGenerator{
					{
						Clusters: &v1alpha1.ClusterGenerator{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"argocd.argoproj.io/secret-type": "cluster",
								},
							},
						},
					},
				},
			},
		}).Then().Expect(ApplicationsExist([]argov1alpha1.Application{expectedAppCluster1, expectedAppCluster2})).

		// Update the ApplicationSet template namespace, and verify it updates the Applications
		When().
		DeleteClusterSecret("my-secret2").
		Then().Expect(ApplicationsExist([]argov1alpha1.Application{expectedAppCluster1})).
		Expect(ApplicationsDoNotExist([]argov1alpha1.Application{expectedAppCluster2})).

		// Delete the ApplicationSet, and verify it deletes the Applications
		When().
		Delete().Then().Expect(ApplicationsDoNotExist([]argov1alpha1.Application{expectedAppCluster1}))
}
