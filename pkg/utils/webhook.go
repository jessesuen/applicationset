package utils

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/argoproj-labs/applicationset/api/v1alpha1"
	"github.com/argoproj-labs/applicationset/common"
	argosettings "github.com/argoproj/argo-cd/v2/util/settings"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	log "github.com/sirupsen/logrus"
	"gopkg.in/go-playground/webhooks.v5/github"
	"gopkg.in/go-playground/webhooks.v5/gitlab"
)

type WebhookHandler struct {
	namespace string
	github    *github.Webhook
	gitlab    *gitlab.Webhook
	client    client.Client
}

func NewWebhookHandler(namespace string, argocdSettingsMgr *argosettings.SettingsManager, client client.Client) (*WebhookHandler, error) {
	// register the webhook secrets stored under "argocd-secret" for verifying incoming payloads
	argocdSettings, err := argocdSettingsMgr.GetSettings()
	if err != nil {
		return nil, fmt.Errorf("Failed to get argocd settings: %v", err)
	}
	githubHandler, err := github.New(github.Options.Secret(argocdSettings.WebhookGitHubSecret))
	if err != nil {
		return nil, fmt.Errorf("Unable to init GitHub webhook: %v", err)
	}
	gitlabHandler, err := gitlab.New(gitlab.Options.Secret(argocdSettings.WebhookGitLabSecret))
	if err != nil {
		return nil, fmt.Errorf("Unable to init GitLab webhook: %v", err)
	}

	return &WebhookHandler{
		namespace: namespace,
		github:    githubHandler,
		gitlab:    gitlabHandler,
		client:    client,
	}, nil
}

func (h *WebhookHandler) HandleEvent(payload interface{}) {
	webURL, revision, touchedHead := getPayloadInfo(payload)
	log.Infof("Received push event repo: %s, revision: %s, touchedHead: %v", webURL, revision, touchedHead)

	appSetList := &v1alpha1.ApplicationSetList{}
	err := h.client.List(context.Background(), appSetList, &client.ListOptions{})
	if err != nil {
		log.Errorf("Failed to list applicationsets: %v", err)
		return
	}

	urlObj, err := url.Parse(webURL)
	if err != nil {
		log.Errorf("Failed to parse repoURL '%s'", webURL)
		return
	}
	regexpStr := `(?i)(http://|https://|\w+@|ssh://(\w+@)?)` + urlObj.Hostname() + "(:[0-9]+|)[:/]" + urlObj.Path[1:] + "(\\.git)?"
	repoRegexp, err := regexp.Compile(regexpStr)
	if err != nil {
		log.Errorf("Failed to compile regexp for repoURL '%s'", webURL)
		return
	}

	for _, appSet := range appSetList.Items {
		for _, gen := range appSet.Spec.Generators {
			// check if the ApplicationSet uses the git generator that is relevant to the payload
			if gen.Git != nil && gitGeneratorUsesURL(gen.Git, revision, repoRegexp) && genRevisionHasChanged(gen.Git, revision, touchedHead) {
				err := refreshApplicationSet(h.client, &appSet)
				if err != nil {
					log.Errorf("Failed to refresh ApplicationSet '%s' for controller reprocessing", appSet.Name)
					continue
				}
			}
		}
	}
}

func (h *WebhookHandler) Handler(w http.ResponseWriter, r *http.Request) {
	var payload interface{}
	var err error

	switch {
	case r.Header.Get("X-GitHub-Event") != "":
		payload, err = h.github.Parse(r, github.PushEvent)
	case r.Header.Get("X-Gitlab-Event") != "":
		payload, err = h.gitlab.Parse(r, gitlab.PushEvents, gitlab.TagEvents)
	default:
		log.Debug("Ignoring unknown webhook event")
		http.Error(w, "Unknown webhook event", http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Infof("Webhook processing failed: %s", err)
		status := http.StatusBadRequest
		if r.Method != "POST" {
			status = http.StatusMethodNotAllowed
		}
		http.Error(w, fmt.Sprintf("Webhook processing failed: %s", html.EscapeString(err.Error())), status)
		return
	}

	h.HandleEvent(payload)
}

func parseRevision(ref string) string {
	refParts := strings.SplitN(ref, "/", 3)
	return refParts[len(refParts)-1]
}

func getPayloadInfo(payload interface{}) (webURL, revision string, touchedHead bool) {
	switch payload := payload.(type) {
	case github.PushPayload:
		webURL = payload.Repository.HTMLURL
		revision = parseRevision(payload.Ref)
		touchedHead = payload.Repository.DefaultBranch == revision
	case gitlab.PushEventPayload:
		webURL = payload.Project.WebURL
		revision = parseRevision(payload.Ref)
		touchedHead = payload.Project.DefaultBranch == revision
	}

	return webURL, revision, touchedHead
}

func genRevisionHasChanged(gen *v1alpha1.GitGenerator, revision string, touchedHead bool) bool {
	targetRev := parseRevision(gen.Revision)
	if targetRev == "HEAD" || targetRev == "" { // revision is head
		return touchedHead
	}

	return targetRev == revision
}

func gitGeneratorUsesURL(gen *v1alpha1.GitGenerator, webURL string, repoRegexp *regexp.Regexp) bool {
	if !repoRegexp.MatchString(gen.RepoURL) {
		log.Debugf("%s does not match %s", gen.RepoURL, repoRegexp.String())
		return false
	}

	log.Debugf("%s uses repoURL %s", gen.RepoURL, webURL)
	return true
}

func refreshApplicationSet(c client.Client, appSet *v1alpha1.ApplicationSet) error {
	// patch the ApplicationSet with the refresh annotation to reconcile
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := c.Get(context.Background(), types.NamespacedName{Name: appSet.Name, Namespace: appSet.Namespace}, appSet)
		if err != nil {
			return err
		}
		if appSet.Annotations == nil {
			appSet.Annotations = map[string]string{}
		}
		appSet.Annotations[common.AnnotationGitGeneratorRefresh] = "true"
		return c.Patch(context.Background(), appSet, client.Merge)
	})
}
