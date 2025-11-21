# Docker-in-Kubernetes with NixOS and deskrun

## Overview

This document explains how deskrun enables Docker-in-Kubernetes using NixOS and nix-env, providing a reliable and flexible approach for running containerized workloads within GitHub Actions runners on local Kubernetes clusters.

## Problem Statement

Running Docker within GitHub Actions runners that are themselves running in Kubernetes containers presents several challenges:

1. **Docker daemon startup**: The Docker daemon requires privileged access and proper cgroup setup
2. **Storage drivers**: Traditional storage drivers may conflict with Kubernetes cgroup configurations
3. **Image management**: Docker needs to manage images across runner instances
4. **Socket accessibility**: The Docker socket must be accessible from the job containers

## Solution: NixOS + nix-env Docker

Instead of using `docker:dind` images (which have namespace isolation issues in Kubernetes), deskrun uses a NixOS base image with Docker installed via `nix-env`:

```dockerfile
# Base: nixos/nix:latest
# Install: docker via nix-env -i docker
# Run: dockerd with overlay2 storage driver
```

### Why NixOS?

1. **Package Management**: Nix provides reliable, reproducible Docker installation
2. **Minimal Dependencies**: NixOS base image is lightweight and well-maintained
3. **No Daemon Conflicts**: Unlike docker:dind, nix-env Docker starts as a simple daemon process
4. **Flexible Configuration**: Can install any tool via nix-env without package conflicts

## Implementation in deskrun

### Container Mode: `cached-privileged-kubernetes`

Use deskrun's privileged mode with NixOS containers:

```bash
deskrun add docker-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --auth-type pat \
  --auth-value ghp_your_token
```

### Architecture

#### 1. Runner Pod (Non-Privileged)

The runner container runs as a non-root user (UID 1001):

```yaml
containers:
  - name: runner
    image: ghcr.io/actions/actions-runner:latest
    securityContext:
      runAsUser: 1001
      runAsGroup: 1001
      allowPrivilegeEscalation: false
    env:
      - name: ACTIONS_RUNNER_CONTAINER_HOOKS
        value: "/home/runner/k8s-novolume/index.js"
      - name: ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE
        value: "/etc/hooks/content"
```

#### 2. Job Container (Privileged via Hook Extension)

When a job runs, ARC's hook extension injects privileged context:

```yaml
# Hook extension ConfigMap patch
containers:
  - name: "$job"
    securityContext:
      privileged: true
      runAsUser: 0
      runAsGroup: 0
    volumeMounts:
      - name: sys
        mountPath: /sys
      - name: cgroup
        mountPath: /sys/fs/cgroup
        mountPropagation: Bidirectional
      - name: dev
        mountPath: /dev
      - name: shm
        mountPath: /dev/shm
```

#### 3. Docker Daemon Setup

Inside the job container with NixOS base:

```bash
# Install Docker via nix
nix-env -i docker

# Start daemon with overlay2 storage driver
dockerd --storage-driver overlay2 &

# Wait for daemon to be ready
while ! docker info >/dev/null 2>&1; do
  sleep 1
done
```

### Key Configuration Details

#### Container Mode: `kubernetes-novolume`

Uses Kubernetes without persistent volumes:
- Ephemeral storage for job artifacts
- No PVC coordination overhead
- Hook extension handles volume mounts

#### Hook Extension Features

The `privileged-hook-extension` ConfigMap provides:
- **Privilege Escalation**: Full privileged context for job containers
- **System Access**: /sys, /cgroup, /proc, /dev mounting
- **Bidirectional Mount Propagation**: Allows nested containerization
- **Capabilities**: SYS_ADMIN, NET_ADMIN, SYS_PTRACE, and others

#### Host Path Caching

For performance, cache directories can be mounted from the host:

```bash
deskrun add docker-runner \
  --cache /var/lib/docker \
  --cache /root/.cache
```

Maps to: `/tmp/github-runner-cache/{runner-name}/cache-{index}`

## Workflow Integration

### Example Workflow with Docker

```yaml
jobs:
  build:
    runs-on: docker-runner
    container:
      image: nixos/nix:latest
      options: --privileged
    steps:
      - name: Install Docker
        run: nix-env -i docker

      - name: Start Docker daemon
        run: |
          dockerd --storage-driver overlay2 &
          while ! docker info >/dev/null 2>&1; do
            sleep 1
          done

      - name: Build Docker image
        run: |
          docker build -t myapp:latest .
          docker run --rm myapp:latest /bin/sh -c "echo 'Success!'"
```

## Performance Considerations

### Docker Layer Caching

Cache the `/var/lib/docker` directory for faster builds:

```bash
deskrun add docker-runner \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker
```

This:
- Preserves Docker images across runs
- Reduces download time for base images
- Speeds up Docker build layer caching

### Multiple Instances

For better cache isolation, use multiple runner instances:

```bash
deskrun add docker-runner \
  --instances 3 \
  --cache /var/lib/docker
```

Each instance gets dedicated cache: `cache-docker-runner-1`, `cache-docker-runner-2`, `cache-docker-runner-3`

## Comparison with Alternatives

### vs. docker:dind

| Aspect | docker:dind | NixOS + nix-env |
|--------|-----------|-----------------|
| Namespace Issues | Problematic in K8s | Not applicable |
| Storage Driver | Limited flexibility | Full overlay2 support |
| Setup Complexity | Image-based | Simple daemon startup |
| Cache Management | Per-container | Host path mounts |
| Build Isolation | Good | Excellent |

### vs. DinD Mode

deskrun's `dind` mode uses the old docker:dind approach. The NixOS approach is superior:

- **Reliability**: No namespace conflicts
- **Performance**: Direct host cgroup access
- **Flexibility**: Can add any tools via nix-env
- **Debugging**: Daemon logs directly visible

## Troubleshooting

### Docker Daemon Won't Start

**Symptom**: "Cannot connect to Docker daemon" in job logs

**Solution**:
1. Check privileged mode is enabled: `deskrun list | grep docker-runner`
2. Verify hook extension ConfigMap exists:
   ```bash
   kubectl get configmap privileged-hook-extension -n arc-systems
   ```
3. Check job container security context:
   ```bash
   kubectl get pod -n arc-systems -o yaml | grep -A5 "securityContext"
   ```

### Storage Driver Errors

**Symptom**: "overlay2: operation not permitted"

**Solution**:
1. Ensure cgroup v2 is properly delegated (handled by hook extension)
2. Check /sys/fs/cgroup is mounted:
   ```bash
   mount | grep cgroup
   ```
3. Verify Bidirectional mount propagation in hook extension

### Docker Socket Issues

**Symptom**: "Permission denied" when accessing docker.sock

**Solution**:
1. Ensure dockerd is running as root in job container
2. Check socket permissions:
   ```bash
   ls -la /var/run/docker.sock
   ```
3. Job container must run as root or in docker group

### Cache Directory Issues

**Symptom**: "Permission denied" on cache mount

**Solution**:
1. Ensure cache directories exist on host:
   ```bash
   mkdir -p /tmp/github-runner-cache/docker-runner
   ```
2. Check permissions:
   ```bash
   ls -la /tmp/github-runner-cache/
   ```
3. The runner should have write access

## Implementation Details

### How Hook Extensions Work

1. Runner creates job pod (non-privileged)
2. Hook extension framework intercepts pod creation
3. Hook extension ConfigMap provides patch specification
4. ARC controller merges patch with pod spec
5. Job container runs with privilege escalation

### Container Mode: kubernetes-novolume

- Disables default persistent volume claims
- Relies on hook extension for all volume mounts
- More efficient for ephemeral runners
- No PVC lifecycle management

### Volume Mounts in Hook Extension

The hook extension provides:

```yaml
volumeMounts:
  - name: sys
    mountPath: /sys
  - name: cgroup
    mountPath: /sys/fs/cgroup
    mountPropagation: Bidirectional
  - name: proc
    mountPath: /proc
  - name: dev
    mountPath: /dev
  - name: dev-pts
    mountPath: /dev/pts
  - name: shm
    mountPath: /dev/shm
```

These enable:
- **systemd operations**: /sys access
- **cgroup manipulation**: /sys/fs/cgroup with bidirectional propagation
- **Device access**: /dev for loop devices, etc.
- **Shared memory**: /dev/shm for IPC

## Best Practices

### 1. Always Use Privileged Mode for Docker

```bash
# ✅ Correct
deskrun add docker-runner \
  --mode cached-privileged-kubernetes

# ❌ Wrong - Docker won't work
deskrun add docker-runner \
  --mode kubernetes
```

### 2. Cache Docker State

```bash
# ✅ Faster builds
deskrun add docker-runner \
  --cache /var/lib/docker

# ❌ Slower - reinstalls images every time
deskrun add docker-runner
```

### 3. Use NixOS Base in Workflows

```yaml
# ✅ Works with Docker
container:
  image: nixos/nix:latest
  options: --privileged

# ❌ May have dependency issues
container:
  image: ubuntu:latest
  options: --privileged
```

### 4. Start Docker Before Use

```bash
# ✅ Always start daemon
dockerd --storage-driver overlay2 &
while ! docker info >/dev/null 2>&1; do sleep 1; done

# ❌ Assumes daemon is pre-running
docker build .
```

### 5. Monitor Cache Sizes

```bash
# Check cache usage
du -sh /tmp/github-runner-cache/*

# Clean old caches
rm -rf /tmp/github-runner-cache/old-runner
```

## References

- [NixOS Docker Package](https://search.nixos.org/packages?channel=unstable&show=docker)
- [GitHub Actions Runner Controller Hook Extensions](https://github.com/actions/actions-runner-controller/blob/master/docs/container-hooks-beta.md)
- [Kubernetes Volume Mount Propagation](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation)
- [cgroup v2 Delegation](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html)

## See Also

- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Common issues and solutions
- [README.md](README.md) - General deskrun documentation
- [EXAMPLES.md](EXAMPLES.md) - Usage examples
