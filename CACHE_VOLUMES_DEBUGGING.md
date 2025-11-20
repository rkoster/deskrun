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

### Test Results - RUN WITHOUT CACHE VOLUMES
**Run 19554987828** - SUCCESSFUL POD INITIALIZATION
- ✓ Set up job
- ✓ Initialize containers  
- ✓ Start Docker daemon
- Pod came online successfully
- Docker daemon started successfully via nix-env
- Failure occurred later in `actions/checkout@v4` step (exit code 127 - different issue)

**CONCLUSION**: Cache volumes ARE the root cause of pod initialization failure.

### Problem Identified
When cache volumes are added to the hook extension ConfigMap, the pod fails with:
```
Error: Pod test-runner-xxx-workflow is unhealthy with phase status Failed: {}
```

This happens during the Kubernetes admission/validation phase before the pod can even start.

## Next Steps

1. ✓ **Confirm cache volumes are the culprit** - CONFIRMED
2. **Identify the specific issue with cache volumes**:
   - Check if paths are invalid or conflicting
   - Verify volume definitions format in hook extension
   - Check if DirectoryOrCreate type is causing issues
   - Verify no duplicate volume definitions
3. **Fix cache volume implementation**:
   - Possibly use different volume types
   - Ensure paths exist on the host before pod creation
   - Use different mounting strategy
4. **Re-enable with fix**

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
