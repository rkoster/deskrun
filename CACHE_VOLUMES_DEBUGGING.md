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
2. **Cache Volumes Are Root Cause**: Confirmed through testing - pods work fine without cache volumes
3. **Not a DirectoryOrCreate Issue**: Manual testing shows `DirectoryOrCreate` type volumes work fine with safe paths
4. **Cache Volumes Only Needed for Job Container**: Cache volumes should only be present in the hook extension patch for job containers, not in the runner pod template

## Root Cause Found (November 21, 2025)

### Manual Testing Results
Created test pods to isolate the issue:

1. **Test 1 - DirectoryOrCreate with safe paths** ✓ SUCCESS
   - Mounted `/tmp/test-cache-0` to `/mnt/cache-0` 
   - Mounted `/tmp/test-cache-1` to `/var/lib/docker-cache`
   - Pod started successfully in Running state
   - **Conclusion**: `DirectoryOrCreate` type works fine

2. **Test 2 - DirectoryOrCreate with `/nix/store`** ✗ FAILED
   - Mounted `/tmp/test-nix-cache` to `/nix/store`
   - Pod entered CrashLoopBackOff
   - Error: `exec: "sleep": executable file not found in $PATH`
   - **Conclusion**: Mounting over `/nix/store` breaks the NixOS image

### Why `/nix/store` Mounting Breaks NixOS

**The `/nix/store` directory contains essential NixOS binaries and libraries.**

When we mount a host path (empty `/tmp/github-runner-cache/...`) to `/nix/store`:
1. The original `/nix/store` from the NixOS image is completely replaced by the mounted path
2. The mounted path is empty or sparse (initial creation)
3. The container can no longer find basic utilities (sleep, sh, nix, etc.)
4. Container init fails because it can't find the shell or required binaries

This is why the pod fails during initialization - the container cannot even start.

## Solution: Detection and Validation

Instead of implementing overlay filesystem logic in deskrun (which is use-case specific), we:

1. **Added validation in `internal/cmd/add.go`** to detect when users try to cache `/nix/store`
2. **Provide helpful error message** directing users to proper solutions:
   - Use opencode-workspace-action with overlayfs support (when implemented)
   - Cache alternative paths like `/root/.cache/nix` for user-level Nix cache
   - Cache `/var/lib/docker` for Docker layer caching (unaffected)

## Recommended Solution for `/nix/store` Caching

For proper `/nix/store` caching support, an overlay filesystem approach is needed:

**OverlayFS Architecture:**
- **Lower layer** (read-only): Original container's `/nix/store` with NixOS binaries
- **Upper layer** (writable): Host cache directory for new/updated packages
- **Merged mount point**: At `/nix/store`, combining both layers

**Implementation Requirements:**
- Init container to set up the overlay mount structure
- Installation of `util-linux` for mount support
- Proper volume mounts for work directory and cache directory
- SYS_ADMIN capability (already present in privileged mode)

This approach is use-case specific and complex, making it better suited for the opencode-workspace-action project where it can be properly integrated with GitHub Actions workflows.

## Decision: Moving to opencode-workspace-action

The overlay filesystem solution for `/nix/store` caching has been documented as a feature request for the opencode-workspace-action project because:

1. **Use-case specific**: The solution is tightly coupled to NixOS workflows in GitHub Actions
2. **Separation of concerns**: deskrun manages the cluster infrastructure; opencode-workspace-action manages the workflow execution environment
3. **Better maintainability**: Solutions specific to GitHub Actions workflows belong in the actions framework
4. **Reusability**: The solution can benefit other projects using opencode-workspace-action

## Files Affected

- `internal/cmd/add.go`: Added validation to detect `/nix/store` caching attempts
- `CACHE_VOLUMES_DEBUGGING.md`: This document

## Testing

- ✓ Manual test confirmed DirectoryOrCreate volumes work with safe mount paths
- ✓ Manual test confirmed mounting to `/nix/store` breaks NixOS (without overlay)
- ✓ Validation error message appears with helpful guidance
- ✓ Code compiles and builds successfully
- ✓ Other cache paths like `/var/lib/docker` continue to work

## Conclusion

The cache volumes issue has been properly diagnosed and addressed:
1. **Root cause identified**: Direct mounting to `/nix/store` overwrites essential NixOS binaries
2. **Validation implemented**: deskrun now prevents this misconfiguration with helpful guidance
3. **Solution documented**: Overlay filesystem support requested in opencode-workspace-action for proper `/nix/store` caching in GitHub Actions workflows

Users can still cache other paths like `/var/lib/docker` and `/root/.cache` without issues.
