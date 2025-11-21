# Investigation: Why Jobs Were Not Being Picked Up by Deskrun Runners

## Problem Statement
After implementing the hook extension pattern for privileged container mode, workflows submitted to deskrun runners were not being picked up. Jobs remained in "queued" state indefinitely without connecting to any runner.

## Root Cause Identified
**The runners were installed BEFORE the recent deskrun code changes, so they do not have `minRunners` and `maxRunners` values in their Helm configuration.**

When the Helm releases were created, they were using an older version of deskrun that did not properly pass these critical values to the ARC Helm chart. The values were in the configuration file but were not being rendered into the Helm values YAML at that time.

## Evidence

### 1. Listener Pod Logs Show Zero Available Jobs
```
"totalAvailableJobs":0
"assigned job": 0, "decision": 0, "min": 0, "max": 2147483647
```

The listener reports:
- **0 total available jobs** - The GitHub API is not assigning jobs to this runner
- **min: 0** - minRunners defaulted to 0 (should be 1)
- **max: 2147483647** - maxRunners defaulted to max int32 (should be the configured value)

### 2. AutoscalingRunnerSet Missing Fields
When checking the deployed ARS:
```bash
kubectl get autoscalingrunnersets -n arc-systems rubionic-workspace-1 -o yaml
# No minRunners or maxRunners fields found in spec
```

The ARS CRD definition DOES support these fields:
```bash
kubectl get crd autoscalingrunnersets.actions.github.com -o yaml | grep "minRunners\|maxRunners"
# Shows both fields are defined in the CRD schema
```

### 3. Configuration File Has Correct Values
The deskrun configuration file (`~/.deskrun/config.json`) shows:
```json
{
  "installations": {
    "rubionic-workspace": {
      "MinRunners": 1,
      "MaxRunners": 1
    }
  }
}
```

But these values were NOT passed to the Helm chart when the runners were installed.

## Why This Prevents Job Pickup

The GitHub Actions Runner Controller (ARC) uses `minRunners` to determine:
1. How many "idle" runners to keep running waiting for jobs
2. Whether to scale up when jobs are available

When `minRunners` is not set:
- ARC assumes minRunners=0 (no idle runners)
- ARC only scales up after GitHub assigns a job
- But GitHub won't assign a job if no runners are available
- This creates a deadlock: no runners available → no jobs assigned → no runners scaled

With `minRunners=1`:
- ARC keeps 1 runner always running and ready
- GitHub sees the runner is available and assigns pending jobs
- Jobs can be picked up immediately

## Timeline

1. **Earlier**: Runners were installed with old deskrun code that had issues with Helm values generation
2. **Recent commits**: 
   - Implemented hook extension pattern (fixes security and job execution)
   - Fixed hook extension YAML format
   - Removed runner group support (repo-level runners only)
3. **Current**: Runners still have old Helm configuration (no minRunners/maxRunners)
4. **Result**: Jobs queue indefinitely, never assigned to runners

## Solution

The runners must be **reinstalled** with the current deskrun code:

```bash
# Remove old runners
deskrun remove rubionic-workspace

# Install new runners with correct Helm values
deskrun add rubionic-workspace \
  --repository https://github.com/rkoster/rubionic-workspace \
  --mode cached-privileged-kubernetes \
  --cache /nix/store \
  --cache /var/lib/docker \
  --instances 3 \
  --min-runners 1 \
  --max-runners 1 \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

## Code Changes Made

### 1. Hook Extension Format Fix
**File**: `internal/runner/runner.go` (lines 446-551)

Changed from invalid custom format to proper Kubernetes PodSpec patch:
- Before: `version: 1, container: {...}`
- After: `spec: { containers: [{ name: "$job", ... }] }`

### 2. Added Test for Min/Max Runners
**File**: `internal/runner/runner_test.go` (new TestGenerateHelmValues_MinMaxRunners)

Added comprehensive test to verify minRunners and maxRunners are always included in generated Helm values.

## Verification

After reinstalling runners, verify the fix:

```bash
# Check ARS has minRunners and maxRunners
kubectl get autoscalingrunnersets -n arc-systems rubionic-workspace-1 -o yaml | grep -i "min\|max"
# Should show:
# - minRunners: 1
# - maxRunners: 1

# Check listener sees available jobs
kubectl logs -n arc-systems rubionic-workspace-1-xxxxx-listener --tail=20 | grep "totalAvailableJobs"
# Should show non-zero when jobs are pending
```

## Lessons Learned

1. **Always test full installation**: Code changes to Helm values generation must be tested end-to-end with actual Kubernetes deployments
2. **Helm values are critical**: Missing Helm values can silently fail without obvious error messages
3. **Default values matter**: When fields are missing, Kubernetes/ARC uses defaults that may not be sensible (max int32 for maxRunners)
4. **Version management**: Code changes require reinstalling resources that were created with the old code

## Related Issues

- Hook extension format was also incorrect initially (fixed in commit e1e8222)
- Runner groups were being added for repo-level runners unnecessarily (fixed in commit 322147d)
- The underlying ARC v0.13.0 bug with empty secrets is avoided by the hook extension pattern

## Files Modified

- `internal/runner/runner.go` - Hook extension ConfigMap generation
- `internal/runner/runner_test.go` - Added min/max runner verification test

## Next Steps

1. Reinstall the rubionic-workspace runners with current deskrun code
2. Verify jobs are now being picked up by the runners
3. Monitor listener logs to confirm job assignment is working
4. Test workflow execution to ensure privileged mode works correctly
