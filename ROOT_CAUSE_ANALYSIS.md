# Root Cause Analysis: Ephemeral Runner Pods Exiting Immediately

## Summary
Ephemeral runner pods created by ARC v0.13.0 are exiting with exit code 0 within 1-2 seconds of starting, preventing them from connecting to GitHub Actions and accepting jobs. This leaves queued jobs stuck in "queued" state forever.

## Root Cause
**The ARC controller is creating Kubernetes secrets without populating them with the JIT configuration data**, creating a critical race condition:

1. ARC controller creates a Kubernetes Secret (empty)
2. ARC controller creates a Pod that references this secret
3. Pod starts and Kubernetes mounts the secret as an environment variable
4. Pod container starts and immediately tries to read `ACTIONS_RUNNER_INPUT_JITCONFIG` from the mounted secret
5. **Secret is still empty** - JIT data hasn't been written yet
6. Runner container exits with exit code 0 (graceful failure, not an error)
7. ARC controller detects exit code 0 and marks the pod as "finished successfully"
8. ARC controller then cleans up the pod and deletes the empty secret
9. The secret is deleted before it's ever populated with data

## Evidence

### Timeline from ARC Controller Logs
All timestamps are in chronological order (same second or +1-2 seconds apart):

```
20:44:38Z Created ephemeral runner secret (empty)
20:44:38Z Created ephemeral runner pod (references the empty secret)
20:44:39Z Ephemeral runner container is still running
20:44:40Z Ephemeral runner has finished successfully, exitCode: 0
20:44:40Z Cleaning up the runner jitconfig secret
```

### Secret Data Inspection
When monitoring active pods and their referenced secrets:
- Secret exists and pod is created in Pending status
- Secret `.data` field is **completely empty** (0 keys instead of 4 expected)
- Pod immediately exits because it cannot read the required environment variable
- Secret is deleted as part of cleanup

### Pod Environment Variable Definition
```
ACTIONS_RUNNER_INPUT_JITCONFIG:
  <set to the key 'jitToken' in secret 'rubionic-workspace-3-b28w7-runner-xxxx'>
  Optional: false
```

The pod requires a non-optional environment variable from a secret that doesn't have any data in it.

## Controller Logs Show No Data Population
Searching through controller logs shows:
- `Creating ephemeral runner JIT config` ✓
- `Created ephemeral runner JIT config` ✓
- `Creating new secret for ephemeral runner` ✓
- `Created ephemeral runner secret` ✓
- **NO LOG** for "Populating secret with JIT data" or similar
- **NO EVIDENCE** of the JIT config data being written to the secret

## Likely Root Cause: ARC v0.13.0 Bug
This appears to be a bug in the GitHub Actions Runner Controller v0.13.0 where:

1. The JIT configuration is retrieved from GitHub API
2. A secret is created to hold the JIT config
3. **But the secret is never populated with the actual JIT data**
4. The pod is created before the data would be available
5. The pod fails silently (exit 0)

## Why Exit Code 0?
The runner exiting with exit code 0 suggests it's not crashing - it's likely:
- Checking for required environment variables
- Finding them empty/unavailable
- Gracefully shutting down without connection
- This is a silent failure, not an exception

## Impact
- Queued GitHub Actions jobs remain stuck indefinitely
- No error messages or indicators that runners are failing
- The system appears to be working but isn't actually accepting jobs
- Only happens with "cached-privileged-kubernetes" mode (where we provide custom templates)

## Potential Solutions

### 1. Upgrade ARC Controller
This may be fixed in a newer version of ARC (> v0.13.0)

### 2. Add Startup Delay/Retry in Runner Template
Modify the pod spec to delay container start or add retry logic:
- Add init container that waits for secret data
- Add health check to detect missing environment variables
- Increase pod startup timeout

### 3. Wait for Secret Population
Modify ARC configuration to wait for the secret to be populated before creating the pod

### 4. Use Different Container Mode
Switch from custom `cached-privileged-kubernetes` template to standard `kubernetes` mode

## Files Involved
- `/home/ruben/workspace/deskrun/internal/runner/runner.go` - Generates Helm values for ARC
- ARC Chart: `gha-runner-scale-set` - Deploys the actual controller
- Kubernetes Secret: `rubionic-workspace-3-b28w7-runner-*` - Created empty, never populated
- Kubernetes Pod: `rubionic-workspace-3-b28w7-runner-*` - Exits immediately due to missing config

## Test Results
- Multiple runs show consistent pattern: every pod exits within 1-2 seconds
- Secret exists but is always empty when pod needs it
- No runner ever successfully connects to GitHub
- No errors or warnings in pod logs (pod deleted too quickly to capture)

## Next Steps
1. Check ARC GitHub repository for known issues/fixes
2. Test with newer ARC version
3. Add debugging/wait logic to pod template
4. Consider workaround in deskrun code if ARC bug is confirmed
