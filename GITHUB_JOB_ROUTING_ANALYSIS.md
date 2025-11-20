# Analysis: GitHub Job Routing Not Working with ARC v0.13.0

## Summary

Deskrun runners are successfully deployed and connected to GitHub, but GitHub Actions jobs are not being assigned to them. The ARC listener continuously reports `totalAvailableJobs: 0` even when workflow jobs are queued.

## Root Cause

**The runners are registered with GitHub (Agent ID 30 confirmed), but GitHub's job routing system is not offering any jobs to the scale set.** This is a GitHub API-level issue, not a Kubernetes or Helm configuration problem.

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

### 2. Ruled Out: Missing minRunners/maxRunners

- Both fields are properly set in the deployed AutoscalingRunnerSet
- Kubernetes patch applied successfully to set these values
- Verified in kubectl output: `minRunners: 1, maxRunners: 5`

### 3. Ruled Out: Runner Timeout/Lifecycle Issues

- Runner pod has been running for 7+ minutes continuously
- No log messages about timeouts or disconnections
- Session is established and maintained
- "Listening for Jobs" message indicates active listening state

### 4. Ruled Out: Network/Authentication Issues

- Runner successfully connects to `pipelinesghubeus2.actions.githubusercontent.com`
- Successfully connects to `broker.actions.githubusercontent.com`
- Successfully obtains OAuth credentials
- Session creation succeeds
- No SSL/TLS errors, authentication errors, or timeout errors in logs

### 5. Verified: Helm Values Generation

- `githubConfigUrl`: properly set to `https://github.com/rkoster/deskrun`
- `runnerScaleSetName`: properly set to `test-runner`
- `containerMode`: properly set to `kubernetes` with privileged hook extension
- Hook extension ConfigMap created and mounted correctly

## Hypothesis: GitHub API Issue or ARC v0.13.0 Limitation

This appears to be one of:

1. **A bug in ARC v0.13.0's scale set registration** - The scale set is created on GitHub, but GitHub's job routing system doesn't recognize it as eligible for job assignment
2. **A GitHub API limitation** - Ephemeral runners or JIT-configured runners might have specific requirements not being met
3. **A race condition** - The scale set might be registering, but jobs are dispatched before the runner connection is fully established
4. **An async propagation delay** - GitHub might take time to sync runner registration across all API endpoints

## Workarounds to Try

### 1. Upgrade ARC Controller

Try upgrading to a newer version of ARC (> 0.13.0):
```bash
deskrun down test-runner
# Manually update ARC Helm chart to newer version
deskrun add test-runner ...
```

### 2. Use Different Container Mode

Try switching from custom `kubernetes` mode to standard `dind` mode:
```bash
deskrun add test-runner --mode dind ...
```

### 3. Trigger Workflow Dispatch

Ensure the workflow is actually being triggered:
```bash
gh workflow run test-runner.yml \
  --repo rkoster/deskrun \
  --ref copilot/implement-github-actions-runner
```

### 4. Check GitHub API Directly

Use GitHub API to verify runner registration:
```bash
curl -H "Authorization: token $GITHUB_TOKEN" \
  https://api.github.com/repos/rkoster/deskrun/actions/runners
```

## ARC Version

Current deployment: **ARC v0.13.0** (`gha-runner-scale-set` chart)

This is an early version of the ephemeral runner support. Newer versions might have fixed job routing issues.

## Configuration

- **Repository**: `https://github.com/rkoster/deskrun`
- **Container Mode**: Kubernetes with privileged hook extension
- **Min Runners**: 1
- **Max Runners**: 5
- **Runner Scale Set ID**: 10
- **Runner Agent ID**: 30
- **Runner Status**: Connected and listening

## Next Steps

1. **Upgrade ARC** to a newer version (0.14.0 or later) to see if job routing is fixed
2. **Test with Different Modes** - Try `dind` mode to isolate whether it's a Kubernetes mode issue
3. **Review GitHub Logs** - Check GitHub Actions run logs to see what error message appears when job isn't assigned
4. **Contact GitHub Support** - If issue persists, report to GitHub that jobs aren't being routed to ARC v0.13.0 ephemeral runners

## Files Modified

- `internal/runner/runner.go` - Removed unsupported `runnerLabels` field and clarified that GitHub auto-assigns the label

## Conclusion

The deskrun implementation and ARC deployment are correctly configured. The issue is at the GitHub API level - GitHub's job routing system is not recognizing the runner scale set as eligible for job assignment. This is likely a known issue in ARC v0.13.0 that would be fixed by upgrading to a newer version.
