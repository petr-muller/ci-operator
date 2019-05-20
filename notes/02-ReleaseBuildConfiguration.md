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
8. `config.go:L125`
