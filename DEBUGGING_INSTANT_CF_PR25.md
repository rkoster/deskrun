# Debugging instant-cf PR #25 Container Initialization Failures

## Executive Summary

GitHub Actions workflows in instant-cf PR #25 are failing during container initialization with "Executing the custom container implementation failed" after running for 2+ minutes. The issue appears to be in the container creation/management process, not missing GitHub Actions runner infrastructure.

## Problem Statement

**Failing Workflows:**
- Build and Publish Docker Image
- Validate Manifests

**Error Pattern:**
```
##[error]Executing the custom container implementation failed. Please contact your self hosted runner administrator.
```

**Failure Point:** "Initialize containers" step runs for ~2 minutes, then fails
**Buffer Deprecation Warning:** `(node:68) [DEP0005] DeprecationWarning: Buffer() is deprecated`

## Architecture Overview

### Current Setup
```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────────┐
│   Runner Pod    │───▶│ Container Hooks  │───▶│   Job Container     │
│ (ARC managed)   │    │ k8s-novolume/    │    │ (Nixery image)      │
│                 │    │ index.js         │    │                     │
└─────────────────┘    └──────────────────┘    └─────────────────────┘
```

**Runner Pod:**
- Image: `ghcr.io/actions/actions-runner:latest` 
- Contains container hooks at `/home/runner/k8s-novolume/index.js`
- Managed by deskrun with privileged hook extension

**Job Containers (Failing):**
- `nixery.dev/shell/git/docker/devbox/nix/nodejs_24` (validate-manifests)
- `nixery.dev/shell/git/docker/devbox/nix/findutils/gnugrep/coreutils` (build-and-push)
- Options: `--privileged --init`

## Verified Working Components

### ✅ Deskrun Issue #21 Fix is Deployed and Working
The GitHub Actions workspace directories ARE properly configured:

```yaml
# In hook extension ConfigMap
volumeMounts:
- name: gh-ws-temp
  mountPath: /__w/_temp        # ✅ Present
- name: gh-ws-actions  
  mountPath: /__w/_actions     # ✅ Present
- name: gh-ws-tool
  mountPath: /__w/_tool        # ✅ Present
- name: github-home
  mountPath: /github/home      # ✅ Present
- name: github-flow
  mountPath: /github/workflow  # ✅ Present

volumes:
- name: gh-ws-temp
  emptyDir: {}                 # ✅ Correct volume type
# ... all other workspace volumes present
```

**Deployment Status:** 
- Fix implemented in commits `77abdcb` (Dec 23) and `643a142` (Dec 24)
- ConfigMap `privileged-hook-extension-instant-cf-cached-privileged-kubernetes` verified
- Hook extension properly generated from YTT templates

### ✅ Runner Configuration
**AutoScalingRunnerSet:** `instant-cf-cached-privileged-kubernetes`
```yaml
env:
- name: ACTIONS_RUNNER_CONTAINER_HOOKS
  value: /home/runner/k8s-novolume/index.js     # ✅ Correct path
- name: ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER  
  value: "false"                                # ✅ Privileged mode
```

**Volume Mounts:**
```yaml
volumeMounts:
- mountPath: /nix/store-host
  name: cache-0                    # Maps to /nix/store ✅
- mountPath: /nix/var/nix/daemon-socket-host  
  name: cache-1                    # Maps to /nix/var/nix/daemon-socket ✅
- mountPath: /var/lib/docker
  name: cache-2                    # Maps to /tmp/deskrun-cache/var-lib-docker ⚠️
```

## Container Hook Analysis

### k8s-novolume vs k8s Differences

**k8s-novolume mode** (`/home/runner/k8s-novolume/index.js`):
- Sets `ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER: "false"`
- Eliminates persistent work volume requirements
- Designed for privileged containers with direct filesystem access
- Runs jobs directly in runner container context

**Container Hook Dependencies:**
- **Runtime**: Node.js 18+ with proper Buffer API
- **Packages**: @actions/core, @kubernetes/client-node, js-yaml, shlex, tar-fs
- **System**: fs, path, child_process.spawn, crypto
- **Kubernetes**: Service account with pod management permissions

## Potential Root Causes

### 1. Docker Volume Mount Path Mismatch ⚠️
**Problem:** docker-setup.sh expects `/var/lib/docker-host` but runner mounts to `/var/lib/docker`

```bash
# docker-setup.sh expects:
if [[ -d "/var/lib/docker-host" ]]; then
    log_info "Using persistent docker cache from host volume"
    ln -sf /var/lib/docker-host /var/lib/docker

# But actual runner mount:
mountPath: /var/lib/docker      # Direct mount, not -host suffix
name: cache-2
```

### 2. Nixery Image Compatibility Issues
**Potential Problems:**
- Different filesystem layout expectations
- Missing runtime dependencies in Nixery images
- Different init system behavior with `--init` flag
- Node.js Buffer API deprecation warnings indicating version mismatch

### 3. Container Hook Execution Context
**Unknown:**
- Whether hooks properly create Nixery-based job containers
- If volume mounts are successfully applied to job containers
- What specific error occurs during 2-minute initialization period

### 4. Script Execution Environment
**docker-setup.sh assumptions:**
- Expects to run inside job container (Nixery image)
- Performs privileged operations (bind mounts)
- Requires specific directory structure

## Failed Workflow Details

### Build and Publish (Run ID: 20507531951)
```yaml
container:
  image: nixery.dev/shell/git/docker/devbox/nix/findutils/gnugrep/coreutils
  options: --privileged --init
```
**Failure:** Initialize containers (2min 02s) → Failed

### Validate Manifests (Run ID: 20507532203)  
```yaml
container:
  image: nixery.dev/shell/git/docker/devbox/nix/nodejs_24
  options: --privileged --init
```
**Failure:** Initialize containers (1min 53s) → Failed

## Questions for Live Debugging

### Priority 1: Identify Exact Failure Point
1. **Hook Loading:** Does the runner successfully load `/home/runner/k8s-novolume/index.js`?
2. **Container Creation:** Does the hook successfully create the Nixery job container pods?
3. **Container Initialization:** Does the failure occur during job container startup or script execution?

### Priority 2: Examine Container State
1. **Pod Status:** Are job container pods created but failing to initialize?
2. **Volume Mounts:** Are the GitHub workspace directories properly mounted in job containers?
3. **Script Access:** Can docker-setup.sh be accessed and executed in the job container?

### Priority 3: Validate Assumptions
1. **Volume Paths:** Confirm docker-setup.sh path expectations vs actual mounts
2. **Image Contents:** Verify Nixery images have expected runtime dependencies
3. **Permissions:** Check if privileged operations work in Nixery containers

## Investigation Commands for Live Debugging

### Check Runner Pod Logs
```bash
kubectl logs -n arc-systems instant-cf-cached-privileged-kubernetes-<pod-id>
```

### Monitor Job Container Creation
```bash
kubectl get pods -n arc-systems -w | grep instant-cf
```

### Examine Hook Extension ConfigMap
```bash
kubectl get configmap privileged-hook-extension-instant-cf-cached-privileged-kubernetes -n arc-systems -o yaml
```

### Verify Container Hooks Exist
```bash
kubectl exec -n arc-systems instant-cf-cached-privileged-kubernetes-<pod-id> -- ls -la /home/runner/k8s*/
```

---

## Debugging Session Log

### Session 1: Initial Analysis (2025-12-27)

#### Findings:
1. **Verified deskrun issue #21 fix is deployed** - All GitHub workspace directories properly configured in hook extension ConfigMap
2. **Analyzed container hook architecture** - Runner uses k8s-novolume hooks at `/home/runner/k8s-novolume/index.js`
3. **Examined runner-container-hooks source code** - Found critical dependencies on `tar`, `find`, `chmod`

#### Root Cause #1: Missing `gnutar` in Nixery Images ✅ FIXED

The container hooks require `tar` to copy files to/from the job container pod. The Nixery images were **missing the `gnutar` package**.

**Evidence:**
```bash
$ docker run --rm nixery.dev/shell/git/docker/devbox/nix/findutils/gnugrep/coreutils sh -c "command -v tar"
# Returns nothing - tar is not found!

$ docker run --rm nixery.dev/shell/gnutar sh -c "command -v tar"
/bin/tar
```

**How the Hooks Use tar** (`runner-container-hooks/packages/k8s/src/k8s/index.ts`):
- `execCpToPod` (line 381-470): `tar xf - --no-same-owner -C ...`
- `execCpFromPod` (line 472-567): `tar cf - -C ...`

**Fix Applied:** Added `gnutar` to Nixery image URLs.

---

### Session 2: After gnutar Fix (2025-12-27)

#### New Failures Observed:

**Run 20537811811 (build-and-publish) - FAILED in 12s:**
```
Error: failed to create job pod: Error: HTTP-Code: 409
pods "instant-cf-cached-privileged-kubernetes-qt46h-runner-z-workflow" already exists
```
- **Cause:** Pod naming collision - stale workflow pod from previous failed run still existed
- **Nature:** Transient issue, pod was cleaned up after failure

**Run 20537812059 (validate-manifests) - FAILED in 1m38s:**
```
sh: line 1: find: command not found
sh: /__w/_temp/27b75920-e30e-11f0-a730-2f54e9f15bbd.sh: No such file or directory
Error: failed to run script step: command terminated with exit code 1
```

#### Root Cause #2: Missing `findutils` in validate-manifests Nixery Image

The `validate-manifests.yml` workflow was updated to use:
```yaml
image: nixery.dev/shell/git/docker/devbox/nix/nodejs_24/gnutar
```

But this image is **missing `findutils`** (which provides `find`).

The container hooks use `find` for permission fixing after file copy:
```typescript
// In execCpToPod (line 397-399):
`find ${shlex.quote(containerPath)} -type f -exec chmod u+rw {} \\; 2>/dev/null; ` +
`find ${shlex.quote(containerPath)} -type d -exec chmod u+rwx {} \\; 2>/dev/null`
```

Without `find`, the hooks retry 15 times (1 second apart), then the script file doesn't exist and execution fails.

---

## Required Utilities for Container Hooks

When using Nixery images with GitHub Actions container mode, **ALL these utilities must be present**:

| Utility | Nixery Package | Purpose | Required By |
|---------|----------------|---------|-------------|
| `sh` | `shell` | Script execution | All hooks |
| `tar` | `gnutar` | File copy to/from pod | `execCpToPod`, `execCpFromPod` |
| `find` | `findutils` | Permission fixing | `execCpToPod` |
| `chmod` | `coreutils` | Permission fixing | `execCpToPod` (via find) |
| `cat` | `coreutils` | Alpine detection | `isPodContainerAlpine` |

---

## THE FIX (Updated)

Both workflow files need **all required packages**:

### build-and-publish.yml ✅ (already has findutils)
```yaml
container:
  image: nixery.dev/shell/git/docker/devbox/nix/findutils/gnugrep/coreutils/gnutar
  options: --privileged --init
```

### validate-manifests.yml ❌ (needs findutils added)
```yaml
# Current (broken):
image: nixery.dev/shell/git/docker/devbox/nix/nodejs_24/gnutar

# Fixed:
image: nixery.dev/shell/git/docker/devbox/nix/nodejs_24/gnutar/findutils/coreutils
```

---

## Success Criteria

- [x] Root cause #1 identified: Missing `gnutar` in Nixery images
- [x] Fix #1 applied: Add `gnutar` to Nixery image URLs
- [x] Root cause #2 identified: Missing `findutils` in validate-manifests image
- [ ] Fix #2 applied: Add `findutils` and `coreutils` to validate-manifests image
- [ ] Job containers successfully created from Nixery images
- [ ] Workflow steps run successfully in job containers

---

### Session 3: After gnutar Fix - New Failure (2025-12-27)

#### Run 20537811811 (build-and-push) - Re-run Result

**Initialize containers:** ✅ SUCCESS (got past this step!)
**Checkout repository:** ❌ FAILED

```
env: '/__e/node20/bin/node': No such file or directory
Error: failed to run script step: command terminated with exit code 127
```

#### Root Cause #3: Missing `/__e` (externals) Volume Mount in Hook Extension

The container hooks expect the `/__e` volume to contain runner externals (node20, git, etc.). This is defined in `runner-container-hooks/packages/k8s/src/k8s/utils.ts`:

```typescript
export const CONTAINER_VOLUMES: k8s.V1VolumeMount[] = [
  {
    name: EXTERNALS_VOLUME_NAME,
    mountPath: '/__e'       // <-- This is missing!
  },
  {
    name: WORK_VOLUME,
    mountPath: '/__w'
  },
  {
    name: GITHUB_VOLUME_NAME,
    mountPath: '/github'
  }
]
```

The deskrun hook extension (`pkg/templates/templates/overlay.yaml`) only provides:
- `/__w/_temp`, `/__w/_actions`, `/__w/_tool` (partial work volumes)
- `/github/home`, `/github/workflow` (partial github volumes)

**But NOT:**
- `/__e` (externals - node20, git, etc.)

The hooks copy runner externals to `/mnt/externals` via init container, which then gets mounted at `/__e` in the job container. Without this, actions that use node (like `actions/checkout`) fail.

#### The Fix

Need to add the externals volume to the hook extension in `pkg/templates/templates/overlay.yaml`:

```yaml
volumeMounts:
  # ... existing mounts ...
  - name: externals
    mountPath: /__e

volumes:
  # ... existing volumes ...
  - name: externals
    emptyDir: {}
```

---

### Session 4: Volume Duplication and glibc Theory (2025-12-27)

#### Fix Attempt #3a: Add externals, work, github volumes to hook extension

Added the missing volumes (`externals`, `work`, `github`) to the hook extension in `overlay.yaml`.

**Result:** ❌ FAILED with HTTP 422 - Duplicate volumes

```
Error: failed to create job pod: Error: HTTP-Code: 422
Message: Pod "instant-cf-cached-privileged-kubernetes-8kgxr-runner-t-workflow" is invalid:
  spec.volumes[16].name: Duplicate value: "externals"
  spec.volumes[18].name: Duplicate value: "github"
  spec.containers[0].volumeMounts[16].mountPath: Invalid value: "/__e": must be unique
  spec.containers[0].volumeMounts[18].mountPath: Invalid value: "/github": must be unique
```

**Analysis:** The container hooks runtime already adds `externals`, `work`, and `github` volumes. Adding them to the hook extension creates duplicates.

#### Fix Attempt #3b: Remove the duplicate volumes from hook extension

Removed `externals`, `work`, `github` from the hook extension, relying on container hooks to provide them.

**Result:** ❌ FAILED - Same error as before

```
env: '/__e/node20/bin/node': No such file or directory
Error: failed to run script step: command terminated with exit code 127
```

**Key Observation:** The `rubionic-workspace` runner uses the SAME hook extension configuration and works correctly. The difference is:
- **rubionic-workspace**: Uses pre-built image `ghcr.io/rkoster/opencode-runner:latest`
- **instant-cf**: Uses Nixery image `nixery.dev/shell/git/docker/devbox/nix/...`

#### Root Cause #4 (Theory): glibc/Dynamic Linker Incompatibility

The error `env: '/__e/node20/bin/node': No such file or directory` is **misleading**. The file likely exists, but **cannot be executed** due to missing dynamic linker dependencies.

**Technical Background:**

1. The Node.js binary at `/__e/node20/bin/node` is provided by GitHub Actions runner
2. It's compiled against **glibc** and expects standard library paths:
   - `/lib/x86_64-linux-gnu/libc.so.6`
   - `/lib64/ld-linux-x86-64.so.2` (dynamic linker/interpreter)
   - `/usr/lib/x86_64-linux-gnu/libstdc++.so.6`

3. **Nixery images use Nix store paths** (`/nix/store/...`) for ALL libraries
4. Standard paths like `/lib`, `/lib64`, `/usr/lib` are **empty or missing**

5. When the kernel tries to execute the Node binary:
   - Reads ELF header, finds interpreter path (e.g., `/lib64/ld-linux-x86-64.so.2`)
   - Tries to execute the interpreter
   - **Interpreter doesn't exist** → returns `ENOENT` ("No such file or directory")
   - This error is for the **interpreter**, not the binary itself, but shell reports it confusingly

**Why rubionic-workspace works:**
The `ghcr.io/rkoster/opencode-runner:latest` image is built on a glibc-based distro (likely Ubuntu/Debian), so it has standard library paths populated.

**Why instant-cf fails:**
Nixery images are pure Nix - no FHS (Filesystem Hierarchy Standard) compliance, no `/lib64` with glibc.

#### Validation Plan

To confirm this theory, create a debug pod that:
1. Mounts the externals volume at `/__e`
2. Runs in the Nixery image
3. Attempts to execute `/__e/node20/bin/node --version`
4. Examines the dynamic linker requirements with `file` and `ldd`

```yaml
# Debug pod manifest
apiVersion: v1
kind: Pod
metadata:
  name: nixery-node-debug
  namespace: arc-systems
spec:
  containers:
  - name: debug
    image: nixery.dev/shell/gnutar/findutils/coreutils/file/binutils
    command: ["sleep", "3600"]
    volumeMounts:
    - name: externals
      mountPath: /__e
  initContainers:
  - name: copy-externals
    image: ghcr.io/actions/actions-runner:latest
    command: ["sh", "-c", "cp -r /home/runner/externals/* /mnt/externals/"]
    volumeMounts:
    - name: externals
      mountPath: /mnt/externals
  volumes:
  - name: externals
    emptyDir: {}
```

#### Validation Results ✅ THEORY CONFIRMED

**Test 1: Check if node binary exists**
```bash
$ kubectl exec -n arc-systems nixery-node-debug -- ls -la /__e/node20/bin/
total 95332
-rwxr-xr-x 1 1001 1001 97607264 Dec 27 11:29 node
# ... other files
```
**Result:** ✅ Node binary exists and is 97MB executable

**Test 2: Check binary type and interpreter**
```bash
$ kubectl exec -n arc-systems nixery-node-debug -- file /__e/node20/bin/node
/__e/node20/bin/node: ELF 64-bit LSB executable, x86-64, version 1 (GNU/Linux), 
dynamically linked, interpreter /lib64/ld-linux-x86-64.so.2, ...
```
**Result:** ✅ Binary requires interpreter at `/lib64/ld-linux-x86-64.so.2`

**Test 3: Check if interpreter path exists**
```bash
$ kubectl exec -n arc-systems nixery-node-debug -- ls -la /lib64/
ls: cannot access '/lib64/': No such file or directory
```
**Result:** ✅ `/lib64/` does NOT exist in Nixery image - confirms root cause

**Test 4: Attempt to execute node**
```bash
$ kubectl exec -n arc-systems nixery-node-debug -- /__e/node20/bin/node --version
exec /__e/node20/bin/node: no such file or directory
```
**Result:** ✅ Exact same error as in workflow - **ROOT CAUSE CONFIRMED**

**Test 5: Check standard library paths**
```bash
$ kubectl exec -n arc-systems nixery-node-debug -- ls -la /lib
/lib exists but only contains Nix symlinks (libmagic.so, etc.)
No glibc libraries present

$ kubectl exec -n arc-systems nixery-node-debug -- ls -la /usr/lib
/usr/lib does not exist
```
**Result:** ✅ Standard FHS library paths are empty or missing

#### Root Cause #4: CONFIRMED - glibc/Dynamic Linker Incompatibility

The GitHub Actions runner's Node.js binary (`/__e/node20/bin/node`) is compiled against glibc with interpreter path `/lib64/ld-linux-x86-64.so.2`. Nixery images are pure Nix and don't have FHS-compliant paths.

**The error "no such file or directory" refers to the missing interpreter, NOT the node binary.**

#### Potential Solutions

1. **Use glibc-based container image** - Replace Nixery with Ubuntu/Debian-based image
2. **Add glibc to Nixery image** - `nixery.dev/glibc/...` might provide FHS-compatible paths
3. **Use `steam-run` or similar FHS wrapper** - Nix tooling that creates FHS-compatible environment
4. **Patch Node binary with `patchelf`** - Modify interpreter path to Nix store location
5. **Build custom image** - Pre-built image with both Nix tools and glibc compatibility

---
*Created: 2025-12-27*  
*Updated: 2025-12-27 - Fourth session: glibc/dynamic linker theory for Nixery incompatibility*
*Status: Theory documented, validation pod manifest ready for testing*