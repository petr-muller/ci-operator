# api.ReleaseBuildConfiguration

## Fields

## Validation

### Top-level

1. There needs to be at least one test or image
2. If RPM build location is specified, then RPM build commands need to be too
3. If base RPM images are specified, then RPM build commands need to be too

#### Resources

1. Some resource requests need to be specified
2. Resource requests need to specify a blanket `*` policy
3. All resource requests must satisfy the following:
  * At least one for requests/limits needs to be specified
  * Both requests and limits' `cpu` and `memory` values needs to be a positive
  kubernetes resource quantity
  * No values other than `cpu` and `memory` can be specified

### Build root image

1. Build root image must be present when `images` are specified
2. If build root image is specified, it needs to be exactly one of
   ImageStreamTag reference and Project Image reference. Both cannot be
   specified

### Test steps

1. Test names must be unique. it is not possible to have two tests with a
   same name.
2. Test must have non-empty name
3. Test cannot be called `images`
4. Test name must match `^[a-zA-Z0-9_.-]*$` regex
5. Test must have non-empty `commands`
6. When test has a specified secret, it needs to match a regex
7. When test has a secret with a mount path, it needs to be a valid absolute
   path
8. All tests must have exactly one type.
9. Each test type then needs to meet its own criteria

#### Container tests

1. If the test has `memory_backed_volume`, it needs to be a Kubernetes quantity
2. Base container (`from`) needs to be specified.

#### Template tests

The template test types, in general, need to validate a cluster profile. They
are only matched against a list of known supported profiles. Some templates
also must have release RPMs available via `tag_specification`

### Input configuration

#### Base images

1. A base image cannot be called `root`
2. As a ImageStreamTag reference, its cluster URL needs to be a valid URL
3. As a ImageStreamTag reference, its `tag` value needs to be present

#### Base RPM images

1. A base image cannot be called `root`
2. As a ImageStreamTag reference, its cluster URL needs to be a valid URL
3. As a ImageStreamTag reference, its `tag` value needs to be present

#### Release tag configuration

1. The namespace needs to be present
2. The name needs to be present

### Promotion

#### With tag configuration

1. Namespace must be present in either promotion or tag specification
2. If promotion does not have neither name nor tag, then tag specification must
   have it. (TODO: no longer valid)

#### Without tag configuration

1. Namespace must be present
2. At least one of name and tag must be present
