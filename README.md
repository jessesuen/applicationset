# YAML Implementation

> Docker image: ghcr.io/lorislab/argocd-applicationset:latest

ApplicationSet will search for `*.yaml` files in the git repository.
>Currently only the roor keys of the yaml file could be use for template.

ApplicationSet Example
```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: test
spec:
  generators:
  - git:
      repoURL: git@gitlab.com:1000kit/playground/local/env.git
      revision: test2
      files:
      - path: '*.yaml'
  template:
    metadata:
      name: '{{path.basename}}-test-local'
    spec:
      project: default
      source:
        repoURL: 'https://harbor.1000kit.org/chartrepo/1000kit/'
        targetRevision: '{{version}}'
        chart: '{{chart}}'
        helm:
          releaseName: '{{path.basename}}'
          values: |
            {{values}}
      destination:
        server: https://kubernetes.default.svc
        namespace: 'test'
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
```
Helm chart definition 
```yaml
chart: ping-quarkus
version: ">=0.0.0-0"
values:
  app:
    db:
      enabled: false
    env:
      ENV_TEST_1: 'false'  
```
Template file (globa.yml)
```yaml
values:
  global:
    default_url: localhost
    db:
      admin:
        username: "postgres"
        password: "onecx"
      host: "onecx-db-postgresql"
      port: "5432"
    routing:
      type: ingress
  app:
    env:
      QUARKUS_TKIT_LOG_CONSOLE_JSON: 'false'
```

Build
```
IMAGE_NAMESPACE="ghcr.io/lorislab" DOCKER_PUSH=false make image
docker push ghcr.io/lorislab/argocd-applicationset:latest
```

Local
```
IMAGE_NAMESPACE="k3d-registry.localhost:5000" DOCKER_PUSH=false make image
docker push k3d-registry.localhost:5000/argocd-applicationset:latest
kubectl patch deployment -n argocd argocd-applicationset-controller -p '{"spec":{"template":{"spec":{"containers":[{"name":"argocd-applicationset-controller","image":"k3d-registry.localhost:5000/argocd-applicationset:latest"}]}}}}'
```

# Argo CD ApplicationSet Controller 

The ApplicationSet controller is a Kubernetes controller that adds support for a new custom `ApplicationSet` CustomResourceDefinition (CRD). This controller/CRD enables both automation and greater flexibility when managing Argo CD Applications across a large number of clusters and within monorepos, plus it makes self-service usage possible on multitenant Kubernetes clusters.

The ApplicationSet controller provides the ability:
- To deploy Argo CD Applications to multiple Kubernetes clusters at once
- To deploy multiple Argo CD applications from a single monorepo
- Allows unprivileged cluster users (those without access to the Argo CD namespace) to deploy Argo CD applications without the need to involve cluster administrators in enabling the destination clusters/namespaces
- Best of all, all these features are controlled by only a single instance of an ApplicationSet custom resource, which means no more juggling of multiple Argo CD Application resources to target those multiple clusters/repos!

Unlike with an Argo CD Application resource, which deploys resources from a single Git repository to a single destination cluster/namespace, ApplicationSet uses templated automation to create, modify, and manage multiple Argo CD applications at once. 

If you are loving Argo CD and want to use ApplicationSet's automation and templating to take your usage to the next level, the ApplicationSet controller a shot!

## Example Spec:

```yaml
# This is an example of a typical ApplicationSet which uses the cluster generator.
# An ApplicationSet is comprised with two stanzas:
#  - spec.generator - producer of a list of values supplied as arguments to an app template
#  - spec.template - an application template, which has been parameterized
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: guestbook
spec:
  generators:
  - clusters: {} # This is a generator, specifically, a cluster generator.
  template: 
    # This is a template Argo CD Application, but with support for parameter substitution.
    metadata:
      name: '{{name}}-guestbook'
    spec:
      project: "default"
      source:
        repoURL: https://github.com/argoproj/argocd-example-apps/
        targetRevision: HEAD
        path: guestbook
      destination:
        server: '{{server}}'
        namespace: guestbook
```

The Cluster generator generates parameters, which are substituted into `{{parameter name}}` values within the `template:` section of the `ApplicationSet` resource. In this example, the cluster generates `name` and `server` parameters (containing the name and API URL for the target cluster), which are then substituted into the template's `{{name}}` and `{{server}}` values, respectively.

The parameter generation via multiple sources (cluster, list, git repos), and the use of those values within Argo CD Application templates, is a powerful combination. Learn more about [generators and template](https://argocd-applicationset.readthedocs.io/en/stable/), the [Cluster generator and various other ApplicationSet generators](https://argocd-applicationset.readthedocs.io/en/stable/Generators/), and more, from the ApplicationSet documentation.


## Documentation

Take a look at our introductory blog post, [Introducing the ApplicationSet Controller for Argo CD](https://blog.argoproj.io/introducing-the-applicationset-controller-for-argo-cd-982e28b62dc5).

Check out [the complete documentation](https://argocd-applicationset.readthedocs.io/) for a complete introduction, how to setup and run the ApplicationSet controller, how it interacts with Argo CD, generators, templates, use cases, and more.

## Community

The ApplicationSet controller is a community-driven project. You can reach the Argo CD ApplicationSet community and developers via the following channels:
- Q & A : [Github Discussions](https://github.com/argoproj-labs/applicationset/discussions)
- Chat : [The #argo-cd-appset Slack channel](https://argoproj.github.io/community/join-slack)

We'd love to have you join us!

## Development

Learn more about how to [setup a development environment, build the ApplicationSet controller, and run the unit/E2E tests](https://argocd-applicationset.readthedocs.io/en/latest/Development/).

Our end goal is to provide a formal solution to replace the [app-of-apps](https://argoproj.github.io/argo-cd/operator-manual/cluster-bootstrapping/) pattern. You can learn more about the founding principles of the ApplicationSet controller from [the original design doc](https://docs.google.com/document/d/1juWGr20FQaJmuuTIS8mBFmWWDU422M_FQMuhp5c1jt4/edit?usp=sharing).

This project will initially be maintained separately from Argo CD, in order to allow quick iteration
of the spec and implementation, without tying it to Argo CD releases. No promises of backwards
compatibility are made, at least until merging into Argo CD proper.
