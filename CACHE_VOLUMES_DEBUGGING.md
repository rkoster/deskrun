# Cache Volumes Debugging Notes

## Problem Statement
When cache volumes were introduced to the hook extension for privileged container mode, workflow jobs started failing with:
```
Error: Pod test-runner-xxx-workflow is unhealthy with phase status Failed: {}
```

The pod would fail during the initialization phase before any workflow steps could run.

## Root Cause Investigation

### Successful Run Reference
Run 19554365935 (commit f6781c2) successfully:
- Used `ghcr.io/nixos/nix:latest` container image
- Started Docker daemon via `nix-env -i docker`
- Configured Docker with vfs storage driver
- **Did NOT have cache volumes configured**
- Pod came online successfully and Docker daemon started

### Failed Runs with Cache Volumes
After introducing cache volumes (commits 6ef5030 onwards):
- Same workflow setup
- Added `/nix/store` and `/var/lib/docker` cache volumes
- Pod failed during initialization with no detailed error message
- The pod never reached a state where any workflow steps could execute

## Key Observations

1. **Error Message is Generic**: The Kubernetes error "Pod is unhealthy with phase status Failed: {}" provides no details about what actually failed
2. **Init Container Issue**: The `fs-init` container would start but the pod would fail before the job container could initialize
3. **Not a Tail/Shell Issue**: Early attempts to fix the NixOS image's missing `tail` utility were unnecessary - the actual issue was with the cache volume configuration
4. **Cache Volumes Only Needed for Job Container**: Cache volumes should only be present in the hook extension patch for job containers, not in the runner pod template

## Current Status

### Temporary Fix Applied
Disabled cache volume mounts and definitions in `generateHookExtensionConfigMap()` (lines 568-621 in internal/runner/runner.go) to test if the pod initializes successfully without cache volumes.

This will help confirm that cache volumes are indeed the cause of the pod initialization failure.

## Next Steps

1. **Test without cache volumes**: Run workflow to confirm pod comes online
2. **Identify cache volume issue**: Determine what specifically about the cache volume configuration causes pod init to fail
3. **Fix cache volume implementation**: Either:
   - Change how volumes are defined in the hook extension
   - Use a different mechanism to mount cache paths
   - Ensure volume mount points don't conflict with system requirements
4. **Re-enable with fix**: Restore cache volumes with proper configuration

## Files Affected

- `internal/runner/runner.go`: `generateHookExtensionConfigMap()` - cache volume configuration
- `internal/runner/runner.go`: `generatePrivilegedContainerMode()` - runner template volumes
- `.github/workflows/test-runner.yml`: Test workflow (using ghcr.io/nixos/nix:latest)

## Architecture Notes

The privileged container mode uses ARC's hook extension pattern:
1. Runner pod template defines only the hook-extension ConfigMap and any volumes needed by the runner itself
2. Hook extension ConfigMap contains a PodSpec patch that gets merged into job pod specs
3. Job pod inherits system volumes (sys, cgroup, proc, dev, dev-pts, shm) from the patch
4. Cache volumes should be mounted only in job pods, not runner pods

The issue may be related to how these volumes are being merged or how the paths are being validated at pod creation time.
