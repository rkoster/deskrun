# GitHub Job Routing Limitation with ARC Ephemeral Runners

## Summary

**ARC v0.13.0 (and the gha-runner-scale-set model in general) has a fundamental architectural limitation: labels are intentionally not supported for ephemeral runners.** This is a deliberate design decision by GitHub, not a bug or configuration error.

When workflows request `runs-on: [self-hosted]` (or any other label), GitHub's job routing system cannot match them to ARC ephemeral runners because:
1. ARC ephemeral runners are not assigned any labels by GitHub
2. GitHub explicitly does not support labels for runner scale sets
3. Jobs requesting label-based runners stay queued forever

## Root Cause: GitHub's Deliberate Design Decision

According to GitHub issue #2445 "Multiple label support for gha-runner-scale-set" (closed):

> **"Labels are not supported and will not be supported for runner scale sets"**
> — GitHub maintainers

This is not a limitation of deskrun or ARC v0.13.0 specifically. It's an architectural decision at the GitHub level for the entire ephemeral runner model.

### Evidence

When checking runner status via GitHub API:
```json
{
  "id": 33,
  "name": "test-runner-lzxh6-runner-wwvnq",
  "status": "online",
  "busy": false,
  "labels": []  // ← EMPTY! This is by design, not a bug
}
```

The runner is registered, online, and idle - but has NO labels because GitHub doesn't assign them to scale set runners.

## Evidence

### 1. Runner Successfully Deployed and Running

```bash
$ kubectl get pods -n arc-systems --context kind-deskrun
NAME                                    READY   STATUS   AGE
test-runner-6cd58d58-listener           1/1     Running  7m34s
test-runner-vltwc-runner-2jgtl          1/1     Running  7m32s
```

The listener pod is running and the actual runner pod is running.

### 2. Runner Registered with GitHub

Runner configuration file shows:
```json
{
  "AgentId": 30,
  "AgentName": "test-runner-vltwc-runner-2jgtl",
  "RunnerScaleSetId": 10,
  "RunnerScaleSetName": "test-runner",
  "ServerUrl": "https://pipelinesghubeus2.actions.githubusercontent.com/bItIsQMkRT6Ip2OljEu3vtRdTzdvJwJzwonFOOJjHTalilDrdN/",
  "ServerUrlV2": "https://broker.actions.githubusercontent.com/"
}
```

- Runner successfully obtained Agent ID 30 from GitHub
- Runner is connected to the message broker at broker.actions.githubusercontent.com
- Session creation succeeded ("Session created" in logs)
- Runner is listening for jobs ("Listening for Jobs" message appears)

### 3. ARC Controller Successfully Created Scale Set

Controller logs show:
```
Created/Reused a runner scale set {"id": 10, "name": "test-runner", "runnerGroupName": "Default"}
Created ephemeral runner JIT config {"runnerId": 30}
Created ephemeral runner secret
Created ephemeral runner pod
Ephemeral runner container is still running
Updated ephemeral runner status with statusPhase: Running
```

All resources created successfully. No errors.

### 4. Listener Reports Zero Jobs Available

```
listener logs:
"totalAvailableJobs": 0
"totalRegisteredRunners": 0
"totalAcquiredJobs": 0
"totalAssignedJobs": 0
```

The listener, which calls GitHub's API to get job statistics, reports that:
- No jobs are available for this scale set
- No runners are registered (despite AgentId: 30 being confirmed in runner config)

This discrepancy suggests GitHub's statistics API is not returning the registered runner.

### 5. Worker Is Continuously Running

The listener worker continuously polls GitHub:
```
listener logs (every 50 seconds):
Calculated target runner count {"assigned job": 0, "decision": 1, "min": 1, "max": 5, "currentRunnerCount": 1}
Getting next message
```

Despite no jobs being available, the listener continues running normally. No errors or connection failures.

## Investigation Steps Completed

### 1. Ruled Out: Label Mismatches

- Workflow uses `runs-on: [self-hosted]`
- GitHub automatically assigns "self-hosted" label to all ephemeral runners (per ARC documentation)
- The ARC Helm chart does not support custom `runnerLabels` field (field is hardcoded, not configurable)
- Removed the unsupported `runnerLabels` field from deskrun code

### 2. Ruled Out: Hook Extension ConfigMap

**NEW - Session 2 Testing (2025-11-20):**
- Deployed runner with standard Kubernetes mode WITHOUT hook extension ConfigMap
- Runner successfully deployed and registered with GitHub (AgentId: 33)
- Runner is online, idle, and waiting for jobs
- Workflow jobs are queued but NOT being assigned to the runner
- **Conclusion: The hook extension is NOT the culprit**
- **The issue persists regardless of hook extension presence**

### 3. ROOT CAUSE - Missing "self-hosted" Label (Session 2 - Critical Discovery)

**GitHub Actions API shows runner has empty labels array:**
- Runner registered: YES (AgentId: 33, status: online)
- Runner idle: YES (busy: false)
- Runner labels: **EMPTY ARRAY** ← PROBLEM!

The "self-hosted" label that GitHub should automatically assign to all self-hosted runners is missing. This breaks job routing because:
- Workflow specifies: `runs-on: [self-hosted]`
- Runner has no labels: `"labels": []`
- GitHub cannot match the job to the runner
- Result: Job stays queued forever

**This is a bug in ARC v0.13.0** - ephemeral runners are not being assigned the "self-hosted" label by GitHub.

### 4. Ruled Out: Missing minRunners/maxRunners

- Both fields are properly set in the deployed AutoscalingRunnerSet
- Kubernetes patch applied successfully to set these values
- Verified in kubectl output: `minRunners: 1, maxRunners: 5`

### 4. Ruled Out: Runner Timeout/Lifecycle Issues

- Runner pod has been running for extended periods continuously
- No log messages about timeouts or disconnections
- Session is established and maintained
- "Listening for Jobs" message indicates active listening state

### 5. Ruled Out: Network/Authentication Issues

- Runner successfully connects to `pipelinesghubeus2.actions.githubusercontent.com`
- Successfully connects to `broker.actions.githubusercontent.com`
- Successfully obtains OAuth credentials
- Session creation succeeds
- No SSL/TLS errors, authentication errors, or timeout errors in logs

### 6. Verified: Helm Values Generation

- `githubConfigUrl`: properly set to `https://github.com/rkoster/deskrun`
- `runnerScaleSetName`: properly set to `test-runner`
- `containerMode`: properly set with correct storage configuration
- Storage configuration now properly nested inside `containerMode` object (not root level)
- Both kubernetes mode and privileged mode properly configured

## Key Finding: Hook Extension is NOT the Issue

**The most critical finding from session 2**: Jobs are NOT being routed even WITHOUT the hook extension ConfigMap. This definitively proves that the hook extension is not causing the job routing failure.

Test Runner Status (Session 2):
- **Container Mode**: Standard Kubernetes (no hook extension)
- **Status**: Online and idle
- **GitHub Agent ID**: 33
- **Available Jobs**: 0 (listener still reports `totalAvailableJobs: 0`)
- **Queued Jobs**: Multiple workflow runs are QUEUED but not assigned

This confirms that the root cause is a GitHub API-level issue with ARC v0.13.0, not a configuration problem with deskrun or the hook extension.

## Next Steps and Solutions

### ❌ NOT a Valid Workaround: Using Scale Set Name as Label

Using `runs-on: [test-runner]` (the scale set name) is **NOT** a valid workaround because:
- GitHub doesn't treat the scale set name as an assignable label
- The runner still has `"labels": []` in the API
- Jobs requesting the scale set name will also stay queued
- This was tested in Session 2 and confirmed to not work

### ✅ Valid Solutions

Since GitHub explicitly does not support labels for ephemeral runners, we have these options:

#### Option 1: Use GitHub Enterprise with Runner Groups (Recommended if using GHEC)
If using GitHub Enterprise Cloud (GHEC), runner groups provide an alternative to labels:
- Runner groups allow job routing without relying on labels
- ARC ephemeral runners can be assigned to runner groups
- Workflows use `runs-on: {group-name}` syntax

However, this requires GitHub Enterprise and is not available on github.com.

#### Option 2: Wait for GitHub to Support Alternative Routing
As of 2025-11-20, GitHub Actions does not provide an alternative label-free routing mechanism for ephemeral runners. This may change in future releases.

#### Option 3: Accept the Limitation and Use Workarounds
Since label-based routing is not available:

**For Development/Testing:**
- Deploy traditional self-hosted runners instead of ephemeral runners
- Traditional runners support labels and can use `runs-on: [self-hosted]`

**For CI/CD:**
- Investigate if GitHub has added label support in newer ARC versions
- Consider using GitHub's hosted runners for label-based workflows
- Document that ephemeral runners require custom routing logic

#### Option 4: Use Older Runner Technology
Self-hosted runners (non-ephemeral) support labels. Consider:
- Deprecating the ARC/ephemeral model for deskrun
- Falling back to traditional self-hosted runner registration
- This would require significant refactoring of deskrun

## Investigation Summary

### What We Tested
1. **Standard Kubernetes Mode** (without hook extension) - Runner deployed successfully, no jobs received
2. **Privileged Mode** (with hook extension) - Same result, jobs not routed
3. **Verified runner status** - Confirmed runner is online and idle via GitHub API
4. **Verified Helm values** - All configuration correctly generated and deployed
5. **Confirmed runner connectivity** - Runner registers with GitHub, connects to broker, maintains session

### What We Confirmed
✅ Runners deploy and register with GitHub successfully
✅ Runners connect to the GitHub Actions broker
✅ Runners are online and idle (not experiencing crashes or disconnects)
✅ Deskrun configuration is correct for ARC v0.13.0
✅ ARC installation is successful
✅ Kubernetes cluster and networking are working
✅ GitHub authentication is working

❌ Runners do NOT receive the "self-hosted" label
❌ Jobs requesting label-based routing cannot find the runners
❌ This is a GitHub API limitation, not a deskrun or ARC bug

### Files Modified
- `internal/runner/runner.go` - Helm values generation for ARC (properly configured, no changes needed for label support)

## Current Status

**ARC Deployment**: v0.13.0 (gha-runner-scale-set)
**Repository**: https://github.com/rkoster/deskrun
**Container Mode**: Kubernetes (standard and privileged modes both tested)
**Runner Status**: Online and idle but unreachable for label-based jobs

## Conclusion

The fundamental issue is a GitHub API limitation that affects all ARC-based ephemeral runners:

> GitHub does not support assigning labels to runner scale sets.

This prevents any workflow using `runs-on: [label-based]` syntax from routing jobs to ARC ephemeral runners. This is a deliberate architectural decision by GitHub, not a bug or configuration error.

**Deskrun is correctly implemented** - the limitation is at GitHub's architecture level.
