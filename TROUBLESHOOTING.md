# deskrun Troubleshooting Guide

This guide helps you diagnose and fix common issues with deskrun.

## Common Issues

### 1. Cluster Creation Fails

**Symptom:**
```
Error: failed to create cluster: ...
```

**Possible Causes and Solutions:**

#### Docker not running
```bash
# Check Docker status
docker ps

# Start Docker if needed (Linux)
sudo systemctl start docker

# On macOS, start Docker Desktop
```

#### kind not installed
```bash
# Install kind
# On macOS
brew install kind

# On Linux
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind
```

#### Port conflicts
```bash
# Check if ports are in use
sudo lsof -i :6443
sudo lsof -i :80
sudo lsof -i :443

# Delete existing cluster
deskrun cluster delete
kind delete cluster --name deskrun
```

### 2. Runner Installation Fails

**Symptom:**
```
Error: failed to install runner: failed to apply runner manifest: ...
```

**Possible Causes and Solutions:**

#### Cluster not ready
```bash
# Check cluster status
deskrun cluster status

# Create cluster if needed
deskrun cluster create

# Verify kubectl can connect
kubectl cluster-info --context kind-deskrun
```

#### Invalid authentication token
```bash
# Verify your PAT has correct permissions:
# - repo (full control)
# - workflow (full control)

# Test token
curl -H "Authorization: token ghp_your_token" https://api.github.com/user

# Create new token if needed at:
# https://github.com/settings/tokens/new
```

#### Namespace issues
```bash
# Check namespace exists
kubectl get namespace arc-systems --context kind-deskrun

# Recreate namespace if needed
kubectl delete namespace arc-systems --context kind-deskrun
kubectl create namespace arc-systems --context kind-deskrun
```

#### ARC Controller CRDs missing
```bash
# Check if CRDs are installed
kubectl get crd autoscalingrunnersets.actions.github.com --context kind-deskrun

# If missing, the controller will be automatically installed on first runner add
# You can also manually install it:
helm install arc-controller \
  oci://ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set-controller \
  --namespace arc-systems \
  --create-namespace \
  --kube-context kind-deskrun

# Wait for CRDs to be ready
kubectl wait --for condition=established \
  --timeout=60s \
  crd/autoscalingrunnersets.actions.github.com \
  --context kind-deskrun
```

**Note**: The first time you add a runner, `deskrun` automatically installs the GitHub Actions Runner Controller and CRDs. This process may take 1-2 minutes.

### 3. Runners Not Appearing in GitHub

**Symptom:**
Runners show in `deskrun list` but not in GitHub UI

**Debugging Steps:**

#### Check runner pods
```bash
# List runner pods
kubectl get pods -n arc-systems --context kind-deskrun

# Check pod logs
kubectl logs -n arc-systems -l app=your-runner-name --context kind-deskrun

# Describe pod for events
kubectl describe pod -n arc-systems <pod-name> --context kind-deskrun
```

#### Verify secret
```bash
# Check secret exists
kubectl get secret your-runner-name-secret -n arc-systems --context kind-deskrun

# View secret (base64 decoded)
kubectl get secret your-runner-name-secret -n arc-systems -o yaml --context kind-deskrun
```

#### Check runner scale set
```bash
# Check AutoscalingRunnerSet
kubectl get autoscalingrunnersets -n arc-systems --context kind-deskrun

# Describe for details
kubectl describe autoscalingrunnersets your-runner-name -n arc-systems --context kind-deskrun
```

#### Verify repository URL
```bash
# Check your configuration
deskrun list

# Repository URL should be exact:
# ✅ https://github.com/owner/repo
# ❌ https://github.com/owner/repo.git
# ❌ git@github.com:owner/repo.git
```

### 4. Runners Exit Immediately (Code 0)

**Symptom:**
Runner pods keep restarting, exit code 0

**This is the classic issue mentioned in the problem statement!**

**Solution:**
This should not happen with deskrun as it uses the correct configuration. However, if it does:

```bash
# Check the container mode
deskrun list

# Should be one of:
# - kubernetes
# - cached-privileged-kubernetes  
# - dind

# If you see other modes, recreate:
deskrun remove problematic-runner
deskrun add problematic-runner \
  --repository https://github.com/owner/repo \
  --mode kubernetes \
  --auth-type pat \
  --auth-value ghp_your_token
```

### 5. Permission Denied Errors in Workflows

**Symptom:**
```
Error: permission denied
```

**Solution:**

For operations requiring elevated permissions (Docker, systemd, Nix):

```bash
# Use privileged mode
deskrun remove your-runner
deskrun add your-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --auth-type pat \
  --auth-value ghp_your_token
```

### 6. Docker Not Available in Runner

**Symptom:**
```
Error: Cannot connect to the Docker daemon
```

**Solution:**

Use DinD mode:

```bash
deskrun remove your-runner
deskrun add your-runner \
  --repository https://github.com/owner/repo \
  --mode dind \
  --auth-type pat \
  --auth-value ghp_your_token
```

Or use privileged mode with Docker cache:

```bash
deskrun add your-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --auth-type pat \
  --auth-value ghp_your_token
```

### 7. Slow Nix Builds

**Symptom:**
Nix builds are very slow, downloading everything each time

**Solution:**

Enable Nix store caching:

```bash
deskrun remove your-runner
deskrun add your-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /nix/store \
  --auth-type pat \
  --auth-value ghp_your_token
```

Verify cache is working:
```bash
# Check cache directory size
du -sh /tmp/github-runner-cache/your-runner-name/cache-0

# Should grow over time as Nix builds cache
```

### 8. Out of Disk Space

**Symptom:**
```
Error: no space left on device
```

**Solution:**

#### Clean Docker
```bash
# Remove unused Docker resources
docker system prune -af --volumes
```

#### Clean caches
```bash
# Check cache sizes
du -sh /tmp/github-runner-cache/*

# Remove old runner caches
rm -rf /tmp/github-runner-cache/old-runner-*
```

#### Increase Docker VM disk (macOS)
```
Docker Desktop > Settings > Resources > Disk image size
```

### 9. Cannot Remove Runner

**Symptom:**
```
Error: failed to remove runner from cluster: ...
```

**Solution:**

#### Force delete resources
```bash
# Delete runner scale set directly
kubectl delete autoscalingrunnersets your-runner-name -n arc-systems --context kind-deskrun --force --grace-period=0

# Delete secret
kubectl delete secret your-runner-name-secret -n arc-systems --context kind-deskrun

# Remove from config
deskrun remove your-runner-name
```

#### Delete entire namespace (nuclear option)
```bash
kubectl delete namespace arc-systems --context kind-deskrun --force --grace-period=0

# Remove all installations
# (manually edit ~/.deskrun/config.json to remove installations)
```

### 10. Configuration Corruption

**Symptom:**
```
Error: failed to load config: failed to parse config: ...
```

**Solution:**

```bash
# Backup current config
cp ~/.deskrun/config.json ~/.deskrun/config.json.backup

# View config
cat ~/.deskrun/config.json

# Fix JSON syntax errors or recreate
rm ~/.deskrun/config.json

# Re-add your runners
deskrun add ...
```

## Debugging Commands

### Cluster Information
```bash
# Cluster status
deskrun cluster status

# Kubernetes cluster info
kubectl cluster-info --context kind-deskrun

# Node information
kubectl get nodes --context kind-deskrun
kubectl describe nodes --context kind-deskrun

# All resources in arc-systems namespace
kubectl get all -n arc-systems --context kind-deskrun
```

### Runner Information
```bash
# List runners
deskrun list

# Runner status
deskrun status

# Pod status
kubectl get pods -n arc-systems --context kind-deskrun

# Pod logs (all containers)
kubectl logs -n arc-systems <pod-name> --all-containers --context kind-deskrun

# Follow logs
kubectl logs -n arc-systems -l app=your-runner --follow --context kind-deskrun

# Pod events
kubectl get events -n arc-systems --context kind-deskrun --sort-by='.lastTimestamp'
```

### Resource Usage
```bash
# Node resources
kubectl top nodes --context kind-deskrun

# Pod resources
kubectl top pods -n arc-systems --context kind-deskrun

# Docker disk usage
docker system df

# Cache disk usage
du -sh /tmp/github-runner-cache/*
```

### Network Debugging
```bash
# Check if pods can reach GitHub
kubectl run -it --rm debug --image=nicolaka/netshoot --context kind-deskrun -- bash
# Inside pod:
curl -I https://api.github.com
nslookup api.github.com
```

## Getting Help

### Collect Diagnostics

When reporting issues, collect this information:

```bash
# System info
deskrun cluster status
deskrun list
kubectl version --context kind-deskrun

# Runner details
kubectl get autoscalingrunnersets -n arc-systems -o yaml --context kind-deskrun
kubectl get pods -n arc-systems -o wide --context kind-deskrun

# Logs
kubectl logs -n arc-systems -l app=your-runner --tail=100 --context kind-deskrun

# Events
kubectl get events -n arc-systems --context kind-deskrun

# Config
cat ~/.deskrun/config.json
```

### Reset Everything

If all else fails, complete reset:

```bash
# Remove all runners
for runner in $(deskrun list | grep "Name:" | awk '{print $2}'); do
  deskrun remove "$runner" 2>/dev/null || true
done

# Delete cluster
deskrun cluster delete

# Clean config
rm -rf ~/.deskrun

# Clean caches
rm -rf /tmp/github-runner-cache

# Start fresh
deskrun cluster create
deskrun add your-runner \
  --repository https://github.com/owner/repo \
  --auth-type pat \
  --auth-value ghp_your_token
```

## Known Limitations

### GitHub Job Routing with ARC Ephemeral Runners

**Limitation**: Workflows using `runs-on: [self-hosted]` or other label-based selectors will NOT match ARC ephemeral runners.

**Why**: GitHub explicitly does not support assigning labels to runner scale sets. This is a deliberate architectural decision, not a bug.

**Details**: See [GITHUB_JOB_ROUTING_ANALYSIS.md](GITHUB_JOB_ROUTING_ANALYSIS.md) for complete analysis.

**Impact**: 
- Standard GitHub Actions workflows that expect `runs-on: [self-hosted]` runners cannot use ARC ephemeral runners
- Custom label-based routing (e.g., `runs-on: [my-custom-label]`) does not work with ARC

**Workarounds**:
- Use GitHub Enterprise with runner groups (if available)
- Deploy traditional self-hosted runners instead of ephemeral runners
- Use GitHub-hosted runners for workflows that require label-based routing

**Status**: This limitation exists in ARC v0.13.0 and is not expected to change in the near term, as it's a GitHub API design decision.

1. **Single Cluster**: deskrun manages one kind cluster at a time
2. **Local Only**: Designed for local development, not production
3. **No Cluster Upgrades**: To upgrade kind, delete and recreate cluster
4. **Manual PAT Updates**: Need to remove and re-add runner to update PAT

## References

- [kind Documentation](https://kind.sigs.k8s.io/)
- [GitHub Actions Runner Controller](https://github.com/actions/actions-runner-controller)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Kubernetes Documentation](https://kubernetes.io/docs/)

## Issue Reporting

If you encounter a bug or issue not covered here:

1. Check existing GitHub issues: https://github.com/rkoster/deskrun/issues
2. Collect diagnostic information (see above)
3. Create a new issue with:
   - Description of the problem
   - Steps to reproduce
   - Expected vs actual behavior
   - Diagnostic output
   - Your environment (OS, Docker version, kind version)
