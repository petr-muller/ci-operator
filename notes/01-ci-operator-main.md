# CI Operator

## Top-level procedure

1. Parse options
2. If `verbose` was requested, set up appropriate fields on `flag.CommandLine`
3. If `help` was requested, print help and exit
4. Validate options
5. Complete options
6. Run the payload workflow. The payload workflow is a `Run` method of the
   `options` struct, which is interesting and nice, because then the options
    struct does not need to be passed around.

## Options

### Generic command options
1. `-h/--help`: Show help
2. `-v`: Show verbose output
3. `--dry-run`: Usual dry-run option, avoid doing any modifications to external
   systems

### Inputs
1. `--config`: ci-operator config file path. This input can also be passed as `$CONFIG_SPEC`
2. `--template`: Paths to optional templates to add as steps. Expected to contain at least one `restart=never` pod.
3. `--git-ref`: Use the provided repo revision for testing instead of the one
   provided in `$JOB_SPEC`.
4. `--give-pr-author-access-to-namespace`: If set, then RBAC will be set so
   that the PR author is allowed to view the temporary namespace created by
   the ci-operator.
5. `--as`: Same as in `oc`/`kubectl`, allows to impersonate another user.
6. `--sentry-dsn-path`: Path to a file with a Sentry DSN secret. When set,
   reporting failures to Sentry instance is enabled.



### Namespace controls
1. `--input-hash`: Add something as an input to the hash that determines
   namespace name
2. `--namespace`: Name of the namespace to be used, override hash-based name
3. `--base-namespace`: Namespace to read builds from, defaults to stable
   (**TODO:** Not sure what this means)
4. `--delete-when-idle`: Annotate the namespace to be deleted after given
   duration after no Pod is running in it. The namespace is actually deleted
   by a different component, the NS TTL Controller
5. `--delete-after`: Annotate the namespace to be deleted after given
   duration after it is created. The namespace is actually deleted by a
   different cpmponent, the NS TTL Controller.

### Outputs
1. `--target`: Can be passed multiple times. Select a subset of graph targets
   that will be built.
2. `--print-graph`: Just print out the graph (for digraph utility) that would be
   executed
3. `--promote`: If set, after all targets PASS, promote all images built by
   the run into a separate namespace/imagestream, to be kept (otherwise they
   are deleted when the NS is deleted).
4. `--artifact-dir`: If set, artifacts from tests and templates will be
   fetched to this directory.
5. `--write-params`: If set, some params will be saved to the file to the
   provided path. (**TODO:** Not sure what this does, try.)

### Option validation

There is a `Validate` method which currently does nothing. When such method
would fail, generic failing JUnit would be output with the
`writeFailingJUnit` method. I removed the empty `Validate` in a PR.

### Option completion

Implemented in `Complete` method, it takes care of processing all provided
parameters and creating the needed structures, reading files, etc. When it
fails, a generic failing JUnit is output with the `writeFailingJUnit` method.

#### Reading ci-operator config
First, the ci-operator config struct is loaded from the provided path, using
the `load.Config` method. If the path was provided, the content of the file
is read. If it was not provided, the content of the `CONFIG_SPEC` environment
variable is read. If both fails, the method returns an error. Otherwise, the
content is YAML-unmarshalled into the `api.ReleaseBuildConfiguration` structure.

#### Validating ci-operator config


