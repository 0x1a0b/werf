{% if include.header %}
{% assign header = include.header %}
{% else %}
{% assign header = "###" %}
{% endif %}
Decrypt data from FILE_PATH or pipe.
Encryption key should be in $WERF_SECRET_KEY or .werf_secret_key file

{{ header }} Syntax

```bash
werf helm secret file decrypt [FILE_PATH] [options]
```

{{ header }} Examples

```bash
  # Decrypt secret file
  $ werf helm secret file decrypt .helm/secret/privacy

  # Decrypt from a pipe
  $ cat .helm/secret/date | werf helm secret decrypt
  Tue Jun 26 09:58:10 PDT 1990
```

{{ header }} Environments

```bash
  $WERF_SECRET_KEY  Use specified secret key to extract secrets for the deploy. Recommended way to 
                    set secret key in CI-system. 
                    
                    Secret key also can be defined in files:
                    * ~/.werf_secret_key (globally),
                    * .werf_secret_key (per project)
```

{{ header }} Options

```bash
      --dir='':
            Change to the specified directory to find werf.yaml config
  -h, --help=false:
            help for decrypt
      --home-dir='':
            Use specified dir to store werf cache files and dirs (default $WERF_HOME or ~/.werf)
  -o, --output-file-path='':
            Write to file instead of stdout
      --tmp-dir='':
            Use specified dir to store tmp files and dirs (default $WERF_TMP_DIR or system tmp dir)
```
