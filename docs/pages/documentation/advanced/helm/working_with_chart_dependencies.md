---
title: Working with chart dependencies
sidebar: documentation
permalink: documentation/advanced/helm/working_with_chart_dependencies.html
author: Alexey Igrychev <alexey.igrychev@flant.com>
---

The chart can include arbitrary number of dependencies called **subcharts**.

Subcharts are placed in the directory `.helm/charts/SUBCHART_DIR`. Each subchart in the `SUBCHART_DIR` is a chart by itself with the similar files structure (which can also have own recursive subcharts).

During deploy process werf will render, create and track all resources of all subcharts.

## Managing subcharts with requirements.yaml

For chart developers, it is often easier to manage a single dependency file which declares all dependencies.

A `requirements.yaml` file is a YAML file in which developers can declare chart dependencies, along with the location of the chart and the desired version. For example, this requirements file declares two dependencies:

```yaml
# requirements.yaml
dependencies:
- name: nginx
  version: "1.2.3"
  repository: "https://example.com/charts"
- name: sysdig
  version: "1.4.12"
  repository: "@stable"
```

* The `name` should be the name of a chart, where that name must match the name in that chart's 'Chart.yaml' file.
* The `version` field should contain a semantic version or version range.
* The `repository` URL should point to a **Chart Repository**. Helm expects that by appending `/index.yaml` to the URL, it should be able to retrieve the chart repository's index. The `repository` can be an alias. The alias must start with `alias:` or `@`.

The `requirements.lock` file lists the exact versions of immediate dependencies and their dependencies and so forth.

The `werf helm dependency` commands operate on that file, making it easy to synchronize between the desired dependencies and the actual dependencies stored in the `charts` directory:
* Use [werf helm dependency list]({{ "documentation/reference/cli/werf_helm_dependency_list.html" | relative_url }}) to check dependencies and their statuses.  
* Use [werf helm dependency update]({{ "documentation/reference/cli/werf_helm_dependency_update.html" | relative_url }}) to update `/charts` based on the contents of `requirements.yaml`.
* Use [werf helm dependency build]({{ "documentation/reference/cli/werf_helm_dependency_build.html" | relative_url }}) to update `/charts` based on the `requirements.lock` file.

All Chart Repositories that are used in `requirements.yaml` should be configured on the system. The `werf helm repo` commands can be used to interact with Chart Repositories:
* Use [werf helm repo add]({{ "documentation/reference/cli/werf_helm_repo_add.html" | relative_url }}) to add Chart Repository.
* Use [werf helm repo index]({{ "documentation/reference/cli/werf_helm_repo_index.html" | relative_url }}).
* Use [werf helm repo list]({{ "documentation/reference/cli/werf_helm_repo_list.html" | relative_url }}) to list existing Chart Repositories.
* Use [werf helm repo remove]({{ "documentation/reference/cli/werf_helm_repo_remove.html" | relative_url }}) to remove Chart Repository.
* Use [werf helm repo update]({{ "documentation/reference/cli/werf_helm_repo_update.html" | relative_url }}) to update local Chart Repositories indexes.

werf is compatible with Helm settings, so by default `werf helm dependency` and `werf helm repo` commands use settings from **helm home folder**, `~/.helm`. But you can change it with `--helm-home` option. If you do not have **helm home folder** or want to create another one use `werf helm repo init` command to initialize necessary settings and configure default Chart Repositories.

## Subchart and values

To pass values from parent chart to subchart called `mysubchart` user must define following values in the parent chart:

```yaml
mysubchart:
  key1:
    key2:
    - key3: value
```

In the `mysubchart` these values should be specified without `mysubchart` key:

{% raw %}
```yaml
{{ .Values.key1.key2[0].key3 }}
```
{% endraw %}

Global values defined by the special toplevel values key `global` will also be available in the subcharts:

```yaml
global:
  database:
    mysql:
      user: user
      password: password
```

In the subcharts these values should be specified as always:

{% raw %}
```yaml
{{ .Values.global.database.mysql.user }}
```
{% endraw %}

Only values by keys `mysubchart` and `global` will be available in the subchart `mysubchart`.

**NOTE** `secret-values.yaml` files from subcharts will not be used during deploy process. Although secret values from main chart and additional secret values from cli params `--secret-values` will be available in the `.Values` as usually.
