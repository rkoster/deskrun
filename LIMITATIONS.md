# deskrun Limitations and Design Constraints

## ARC Ephemeral Runners and Job Routing

### The Core Issue

**deskrun currently uses GitHub Actions Runner Controller (ARC) ephemeral runners, which have a fundamental limitation: GitHub does not assign labels to runner scale sets.**

This means workflows using standard GitHub Actions job routing syntax **cannot find or run on deskrun runners**:

```yaml
# ❌ These will NOT work with deskrun (jobs will stay queued forever)
jobs:
  my-job:
    runs-on: [self-hosted]  # Looking for "self-hosted" label - NOT FOUND
    
  another-job:
    runs-on: [custom-label]  # Looking for custom label - NOT FOUND
```

### Why This Limitation Exists

According to GitHub issue #2445 "Multiple label support for gha-runner-scale-set" (closed):

> **"Labels are not supported and will not be supported for runner scale sets"**

This is **not a bug** - it's a deliberate architectural decision made by GitHub. The ephemeral runner model (gha-runner-scale-set) is fundamentally different from traditional self-hosted runners and doesn't support labels.

**Evidence**: When checking runner status via GitHub API, ARC runners have `"labels": []` (empty), and there's no mechanism to assign labels to them.

## Workarounds

### For Development/Testing

If you need to test local runners with standard GitHub Actions workflows:

1. **Use GitHub Enterprise Cloud** (if available)
   - GitHub Enterprise supports runner groups as an alternative to labels
   - Runners can be assigned to groups: `runs-on: {group-name}`
   - Deskrun would need modifications to support runner groups

2. **Manually trigger workflows**
   - Use `workflow_dispatch` to manually run workflows on deskrun runners
   - Workflows can explicitly request deskrun runners without labels

3. **Use separate test workflows**
   - Create test workflows that don't rely on job routing
   - Run them manually with `workflow_dispatch`

### For CI/CD Integration

If you need deskrun runners to automatically receive jobs:

1. **Implement webhook-based routing** (not supported by GitHub Actions natively)
   - This would require significant custom development
   - Not recommended for standard GitHub Actions use

2. **Use traditional self-hosted runners**
   - Switch from ARC ephemeral runners to traditional self-hosted runners
   - Would require replacing deskrun's entire runner management system
   - Traditional runners support labels and `runs-on: [self-hosted]`

3. **Use GitHub-hosted runners for label-based jobs**
   - GitHub-hosted runners are available for standard job routing
   - Use deskrun only for jobs that don't require label-based routing

## Current Capabilities

### What Works ✅

- Deploy runners locally using kind clusters
- Use privileged mode for Docker, systemd, and other elevated operations
- Cache Nix store, Docker daemon, and other directories
- Multiple runner instances with isolated caches
- SSH-like execution environment for testing

### What Doesn't Work ❌

- Standard GitHub Actions job routing (`runs-on: [self-hosted]`)
- Custom label-based job routing (`runs-on: [custom-labels]`)
- Automatic job assignment to deskrun runners
- Integration with GitHub's built-in runner selection

## Design Decisions

### Why ARC Ephemeral Runners?

ARC ephemeral runners were chosen for deskrun because they offer:

1. **Ephemeral execution**: Runners clean up automatically after each job
2. **Isolated environments**: Each job gets a fresh runner pod
3. **Kubernetes-native**: Works with kind, no external VMs needed
4. **JIT configuration**: Secure credential handling without persistent secrets
5. **Hook extensions**: Support for privileged operations via Kubernetes hooks

Traditional self-hosted runners offer:

- ❌ Persistent state between jobs (harder to clean up)
- ❌ Manual setup and configuration
- ❌ No Kubernetes integration
- ✅ **Labels support (this is the trade-off)**

### The Trade-off

**ARC ephemeral runners** are better for:
- Local development environments
- Isolated job execution
- Clean build artifacts between runs
- Security (ephemeral VMs reduce attack surface)

**Traditional self-hosted runners** are better for:
- Production CI/CD with label-based routing
- Long-lived runners that accumulate state
- Standard GitHub Actions integration

## Future Directions

### Option 1: Accept the Limitation

Keep deskrun as a development tool for local testing:
- Document clearly that it's not for production CI/CD
- Provide examples of using `workflow_dispatch` for manual testing
- Users must understand job routing doesn't work

### Option 2: Add Traditional Runner Support

Implement traditional self-hosted runner registration in deskrun:
- Allows label-based job routing to work
- More complex runner lifecycle management
- Requires persistent runner state
- Loses ephemeral runner benefits (cleanup, isolation)

### Option 3: Implement Runner Groups (GHEC Only)

Add support for GitHub Enterprise runner groups:
- Works with ARC ephemeral runners
- Requires GitHub Enterprise Cloud subscription
- Uses `runs-on: {group-name}` syntax
- Better than labels for some use cases

### Option 4: Custom Job Routing

Implement a webhook-based job dispatcher:
- Receives workflow webhooks from GitHub
- Routes jobs to deskrun runners based on custom logic
- Bypasses GitHub's built-in job routing entirely
- Complex to implement and maintain
- Not recommended

## Recommendations

### For Local Development

Use deskrun with `workflow_dispatch`:

```yaml
name: CI

on:
  push:
    branches: [main]
  workflow_dispatch:  # ← Manual trigger

jobs:
  test:
    runs-on: [self-hosted]
    steps:
      - uses: actions/checkout@v4
      - run: ./test.sh
```

When you want to test locally:
```bash
gh workflow run ci.yml --ref main
```

### For Small Teams

If you need automatic job routing:

1. Use GitHub Enterprise with runner groups (if available)
2. Or, use GitHub-hosted runners for production CI/CD

### For Production

Do not use deskrun for production CI/CD pipeline:
- Use GitHub-hosted runners (built-in, no setup needed)
- Or, use traditional self-hosted runners (if you need customization)
- deskrun is designed for development, not production

## Possible Solutions Being Explored

### 1. GitHub May Add Label Support in Future Versions

If GitHub adds label support to ARC runners in a future release:
- Deskrun would work seamlessly with standard job routing
- No code changes needed in deskrun
- Monitor GitHub Actions roadmap for announcements

### 2. Community May Build Solutions

Open-source alternatives being explored:
- Custom GitHub Actions that route jobs to deskrun
- Webhook-based job dispatchers
- Runner group implementations for open-source projects

### 3. Alternative Runner Implementations

Other projects exploring similar local runner concepts:
- Check GitHub's runner ecosystem for new solutions
- Evaluate if they support label-based routing

## Conclusion

**deskrun is currently suitable for local development and testing**, but not for production CI/CD pipelines that rely on standard GitHub Actions job routing.

The limitation is not a bug in deskrun - it's a design constraint of GitHub's ARC ephemeral runner technology.

Users should:
1. Understand this limitation before adopting deskrun
2. Use `workflow_dispatch` for manual testing workflows
3. Use GitHub-hosted runners for production CI/CD
4. Consider GitHub Enterprise runner groups if available

For more details, see:
- [GITHUB_JOB_ROUTING_ANALYSIS.md](GITHUB_JOB_ROUTING_ANALYSIS.md) - Technical analysis
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Known limitations section
