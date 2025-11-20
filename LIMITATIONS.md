# deskrun Usage Guide: ARC Scale Set Name Routing

## How Job Routing Works with Deskrun

**Deskrun uses GitHub Actions Runner Controller (ARC) which routes jobs using scale set names, not labels.** This is the officially supported method.

When you create a runner scale set, you specify a name. GitHub then routes jobs requesting that name to the scale set:

```yaml
# This WORKS with deskrun
jobs:
  build:
    runs-on: my-runner  # ← Scale set name
    steps:
      - run: ./build.sh
```

### Difference from Traditional Self-Hosted Runners

| Feature | Traditional Runners | ARC Runners |
|---------|-------------------|-----------|
| Routing | Labels: `runs-on: [self-hosted, linux]` | Scale set name: `runs-on: my-runner` |
| Label Assignment | Manual per runner | Not supported |
| Multiple Labels | Yes | N/A |
| Router | GitHub API label matching | GitHub scale set name matching |
| On github.com | ✅ Yes | ✅ Yes |
| On GitHub Enterprise | ✅ Yes | ✅ Yes |

## Using Deskrun in Workflows

### 1. Create a Runner

```bash
deskrun add build-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /nix/store \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

### 2. Use the Runner in Workflows

```yaml
name: Build

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: build-runner  # ← Use the scale set name
    steps:
      - uses: actions/checkout@v4
      - run: nix flake check
```

### 3. Create Multiple Runners for Different Purposes

```bash
# Nix-based builds
deskrun add nix-builder \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /nix/store

# Docker-based tests
deskrun add docker-tester \
  --repository https://github.com/owner/repo \
  --mode dind

# Standard builds
deskrun add standard-builder \
  --repository https://github.com/owner/repo \
  --mode kubernetes
```

Then route different jobs to different runners:

```yaml
jobs:
  nix-build:
    runs-on: nix-builder
    steps:
      - run: nix build .

  docker-test:
    runs-on: docker-tester
    steps:
      - run: docker build .

  standard-build:
    runs-on: standard-builder
    steps:
      - run: make build
```

## Benefits of Scale Set Name Routing

1. **Explicit and Clear**: You know exactly which runner will execute the job
2. **No Label Complexity**: No need to manage custom labels or defaults
3. **Simple Naming**: Use descriptive names like "nix-builder", "docker-tester"
4. **Easy to Understand**: New team members don't need to learn label conventions
5. **Supported Officially**: This is GitHub's recommended approach for ARC

## Comparison with Traditional Approach

### Traditional Self-Hosted Runners (Not Used in Deskrun)

```bash
# Traditional runner registration
./config.sh --url https://github.com/owner/repo \
            --token token

# Auto-assigned labels: self-hosted, linux, x64
# You can add custom labels during registration
```

```yaml
jobs:
  build:
    runs-on: [self-hosted, linux, gpu]  # ← Labels
```

### ARC with Deskrun (Recommended)

```bash
deskrun add my-runner  # ← Just a name
```

```yaml
jobs:
  build:
    runs-on: my-runner  # ← Scale set name
```

## FAQ

### Q: Can I use `runs-on: [self-hosted]` with deskrun?

**A**: No. ARC doesn't assign the "self-hosted" label to runners. Use the scale set name instead: `runs-on: my-runner`

### Q: Can I use custom labels like `runs-on: [gpu]`?

**A**: No. ARC doesn't support custom labels. Use scale set names: `runs-on: gpu-runner`

### Q: How do I route jobs to different runners?

**A**: Create multiple runners with different names and reference them in workflows:

```bash
deskrun add gpu-runner ...
deskrun add cpu-runner ...
```

```yaml
jobs:
  heavy-compute:
    runs-on: gpu-runner
    steps: ...
      
  light-compute:
    runs-on: cpu-runner
    steps: ...
```

### Q: What if I want to use `workflow_dispatch` for manual triggering?

**A**: You can still use `workflow_dispatch` alongside automatic routing:

```yaml
on:
  push:
  workflow_dispatch:

jobs:
  build:
    runs-on: my-runner  # Works for both push and manual dispatch
    steps: ...
```

## Design Tradeoffs

### Benefits of ARC Scale Set Naming

✅ **Simpler**: No need to manage labels  
✅ **More Explicit**: Exactly which runner runs the job  
✅ **Cleaner Configuration**: Just a name, not a list  
✅ **Easier for Teams**: Descriptive names vs label conventions  
✅ **Official Support**: GitHub's recommended approach for ARC  

### Benefits of Label-Based Routing (Traditional Runners)

✅ **More Flexible**: Combine multiple labels for selection  
✅ **Automatic Defaults**: `self-hosted` label applied automatically  
✅ **More Granular**: Can filter by OS, architecture, custom labels  
✅ **Familiar**: Standard GitHub Actions pattern  

## Current Capabilities

### What Works ✅

- Deploy runners locally using kind clusters
- Use scale set names for job routing (`runs-on: runner-name`)
- Use privileged mode for Docker, systemd, and other elevated operations
- Cache Nix store, Docker daemon, and other directories
- Multiple runner instances with isolated caches
- SSH-like execution environment for testing
- Both github.com and GitHub Enterprise support

### What Doesn't Work ❌

- Traditional label-based routing (`runs-on: [self-hosted]`)
- Custom labels (`runs-on: [my-label]`)
- Multiple label combinations
- Automatic "self-hosted" label assignment

**Note**: These aren't limitations - they're by design. ARC uses a different (and simpler) routing model.

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
