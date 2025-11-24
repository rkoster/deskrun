# deskrun Examples

This document provides practical examples for using deskrun to set up local GitHub Actions runners.

## Basic Setup

### 1. Simple Repository Runner

For a basic repository that doesn't need special capabilities:

```bash
# Add a runner for your repository
deskrun add my-repo-runner \
  --repository https://github.com/myorg/myrepo \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here

# Check status
deskrun status

# View all installations
deskrun list
```

### 2. Check Cluster Status

```bash
# Check if cluster is running
deskrun cluster status

# Manually create cluster (if needed)
deskrun cluster create

# Delete cluster
deskrun cluster delete
```

## Advanced Configurations

### 3. Docker Build Runner with Multiple Caches

For projects that build Docker images and need multiple caches:

```bash
deskrun add docker-runner \
  --repository https://github.com/myorg/docker-project \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --cache /root/.cache \
  --min-runners 2 \
  --max-runners 10 \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here
```

This configuration:
- Caches Docker daemon data at `/var/lib/docker`
- Caches additional build artifacts at `/root/.cache`
- Scales from 2 to 10 runners

### 3a. Docker Build Runner with Custom Cache Paths

For projects that need persistent cache paths across restarts:

```bash
deskrun add persistent-docker-runner \
  --repository https://github.com/myorg/docker-project \
  --mode cached-privileged-kubernetes \
  --cache /host/persistent/docker:/var/lib/docker \
  --cache /host/persistent/npm:/root/.npm \
  --cache /host/persistent/cargo:/usr/local/cargo/registry \
  --min-runners 2 \
  --max-runners 10 \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here
```

This configuration:
- Uses persistent host paths for Docker, npm, and Cargo caches
- Survives cluster restarts and rebuilds
- Provides better cache persistence than auto-generated paths

### 4. Docker-in-Docker Runner

For clean Docker environments without host contamination:

```bash
deskrun add dind-runner \
  --repository https://github.com/myorg/project \
  --mode dind \
  --min-runners 1 \
  --max-runners 5 \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here
```

This configuration:
- Runs Docker in a sidecar container
- Provides isolated Docker environment
- Good for container-based workflows

### 5. Multiple Runners for Different Repos

You can run multiple runner installations simultaneously:

```bash
# Frontend repository
deskrun add frontend-runner \
  --repository https://github.com/myorg/frontend \
  --mode kubernetes \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here

# Backend repository with Docker
deskrun add backend-runner \
  --repository https://github.com/myorg/backend \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here

# Infrastructure repository
deskrun add infra-runner \
  --repository https://github.com/myorg/infrastructure \
  --mode kubernetes \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here

# List all runners
deskrun list
```

## GitHub App Authentication

For organization-wide deployments, use GitHub App authentication:

### 1. Create a GitHub App

1. Go to GitHub Settings > Developer settings > GitHub Apps
2. Click "New GitHub App"
3. Configure:
   - Name: "My Deskrun Runners"
   - Homepage URL: Your organization URL
   - Webhook: Uncheck "Active"
   - Permissions:
     - Repository permissions:
       - Actions: Read and write
       - Administration: Read and write
       - Metadata: Read-only
4. Generate a private key and download it

### 2. Install the App

1. Install the app to your organization or repositories
2. Note the App ID and Installation ID

### 3. Configure deskrun with GitHub App

```bash
deskrun add org-runner \
  --repository https://github.com/myorg/myrepo \
  --auth-type github-app \
  --auth-value "$(cat path/to/private-key.pem)" \
  --min-runners 5 \
  --max-runners 20
```

## Management Operations

### Updating a Runner

To update a runner configuration, remove and re-add it:

```bash
# Remove old configuration
deskrun remove my-runner

# Add with new configuration
deskrun add my-runner \
  --repository https://github.com/myorg/myrepo \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --max-runners 10 \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here
```

### Viewing Detailed Status

```bash
# Check specific runner status
deskrun status my-runner

# This shows:
# - Runner pods
# - Current scale
# - Recent events
```

### Cleanup

```bash
# Remove a specific runner
deskrun remove my-runner

# Remove all runners and delete cluster
deskrun remove frontend-runner
deskrun remove backend-runner
deskrun cluster delete
```

## Use Cases

### Development Workflow

```bash
# Morning: Start your runners
deskrun cluster create
deskrun add dev-runner \
  --repository https://github.com/myorg/myrepo \
  --mode kubernetes \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here

# Work on your code, push changes, runners execute actions locally

# Evening: Clean up
deskrun remove dev-runner
deskrun cluster delete
```

### CI/CD Pipeline Testing

```bash
# Set up runner for testing pipeline changes
deskrun add test-runner \
  --repository https://github.com/myorg/myrepo \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --auth-type pat \
  --auth-value ghp_your_github_pat_here

# Test your workflow changes
git push origin feature/new-workflow

# Watch logs
kubectl logs -n arc-systems -l app=test-runner --follow

# Clean up when done
deskrun remove test-runner
```

### Multi-Project Development

```bash
# Set up runners for all your active projects
for repo in frontend backend api infrastructure; do
  deskrun add ${repo}-runner \
    --repository https://github.com/myorg/${repo} \
    --mode kubernetes \
    --min-runners 1 \
    --max-runners 3 \
    --auth-type pat \
    --auth-value ghp_your_github_pat_here
done

# Check all runners
deskrun list
deskrun status
```

## Tips and Best Practices

### 1. Cache Paths

Common cache path configurations:

**Auto-generated host paths:**
- `--cache /var/lib/docker` - Docker daemon data (auto-generated host path)
- `--cache /root/.cache` - General application caches
- `--cache /home/runner/.cache` - User-level caches

**Custom host paths for persistence:**
- `--cache /host/persistent/docker:/var/lib/docker` - Persistent Docker cache
- `--cache /host/persistent/npm:/root/.npm` - Persistent npm cache
- `--cache /host/persistent/cargo:/usr/local/cargo/registry` - Persistent Cargo cache
- `--cache /host/build-cache:/tmp/build-cache` - Custom build cache location

**Benefits of custom host paths:**
- Survive cluster restarts and rebuilds
- Shared across multiple runner installations
- Better cache performance with dedicated storage
- Easier backup and management

### 2. Runner Scaling

- **Min Runners**: Set to 1 or 2 for always-ready runners
- **Max Runners**: Set based on your machine's capacity
- Consider your machine's CPU and memory when setting max runners

### 3. Container Modes

Choose the right mode for your needs:

- **kubernetes**: Default, use for most cases
- **cached-privileged-kubernetes**: Use when you need:
  - Nested Docker/containers
  - systemd access
  - cgroup manipulation
- **dind**: Use when you need:
  - Clean Docker environment
  - Isolated Docker daemon
  - Container-based workflows

### 4. Authentication

- Use PAT for personal projects and testing
- Use GitHub App for organization-wide deployments
- Rotate PATs regularly
- Store PATs securely (use password manager or env vars)

### 5. Resource Management

```bash
# Check cache size
du -sh /tmp/github-runner-cache/*

# Clean old caches if needed
rm -rf /tmp/github-runner-cache/old-runner

# Monitor cluster resources
kubectl top nodes
kubectl top pods -n arc-systems
```

## Troubleshooting

For common issues and solutions, see the Troubleshooting section in the main [README.md](README.md).
