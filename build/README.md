# Publishing Helm Chart

## Packaging the Chart

```shell
$ helm package CHART-PATH
```

Replace CHART-PATH with the path to the directory that contains your Chart.yaml file.

Helm uses the chart name and version for the archive file name. In case of opsramp-tracing-proxy it would be similar to
opsramp-tracing-proxy-0.1.0.tgz

## Pushing the Chart to Google Artifact Repository

### Install and Initialize Google Cloud CLI

**Link:** https://cloud.google.com/sdk/docs/install-sdk

### Configure Docker Config for Push

```shell
$ gcloud auth configure-docker REPO-LOCATION
```

REPO-LOCATION can be found [location](https://cloud.google.com/artifact-registry/docs/repositories/repo-locations)

### Pushing the Chart

```shell
$ helm push opsramp-tracing-proxy-0.1.0.tgz oci://LOCATION-docker.pkg.dev/PROJECT/REPOSITORY
```

Replace the following values:

**LOCATION** is the regional or
multi-regional [location](https://cloud.google.com/artifact-registry/docs/repositories/repo-locations) of the
repository.

**PROJECT** is your Google
Cloud [project ID](https://cloud.google.com/resource-manager/docs/creating-managing-projects#identifying_projects). If
your project ID contains a colon (:), see Domain-scoped projects.

**REPOSITORY** is the name of the repository.

### Verify that the push operation was successful

```shell
$ gcloud artifacts docker images list LOCATION-docker.pkg.dev/PROJECT/REPOSITORY
```

## Installing the Helm Chart

### Installing the Chart

```shell
$ helm pull oci://LOCATION-docker.pkg.dev/PROJECT/REPOSITORY/IMAGE \
    --version VERSION \
    --untar

$ helm create ns NAMESPACE

$ cd opsramp-tracing-proxy
$ helm install opsramp-tracing-proxy -n opsramp-tracing-proxy .
```

Replace the following values:

**LOCATION** is the regional or
multi-regional [location](https://cloud.google.com/artifact-registry/docs/repositories/repo-locations) of the
repository.

**PROJECT** is your Google Cloud project ID. If
your [project ID](https://cloud.google.com/resource-manager/docs/creating-managing-projects#identifying_projects)
contains a colon (:), see [Domain-scoped](https://cloud.google.com/artifact-registry/docs/docker/names#domain) projects.

**REPOSITORY** is the name of the repository where the image is stored.

**IMAGE** is the name of the image in the repository.

**VERSION** is semantic version of the chart. This flag is required. Helm does not support pulling a chart using a tag.

