{% if include.header %}
{% assign header = include.header %}
{% else %}
{% assign header = "###" %}
{% endif %}
Generate werf environment variables for specified CI system.

Currently supported only GitLab CI

{{ header }} Syntax

```bash
werf ci-env CI_SYSTEM [options]
```

{{ header }} Examples

```bash
  # Load generated werf environment variables on gitlab job runner
  $ source <(werf ci-env gitlab --tagging-strategy tag-or-branch)
```

{{ header }} Options

```bash
      --docker-config='':
            Specify docker config directory path. WERF_DOCKER_CONFIG or DOCKER_CONFIG or ~/.docker 
            will be used by default (in the order of priority).
  -h, --help=false:
            help for ci-env
      --home-dir='':
            Use specified dir to store werf cache files and dirs (use WERF_HOME environment or 
            ~/.werf by default)
      --tagging-strategy='':
            tag-or-branch: generate auto '--tag-git-branch' or '--tag-git-tag' tag by specified 
            CI_SYSTEM environment variables
      --tmp-dir='':
            Use specified dir to store tmp files and dirs (use WERF_TMP environment or system tmp 
            dir by default)
```
