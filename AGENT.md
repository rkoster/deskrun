# DeskRun Agent Documentation

## Project Mission & Architecture

### Core Purpose

**DeskRun is a highly optimized GitHub Actions runner deployment solution designed specifically for bare metal infrastructure with NVMe caching.** The primary use case is deploying **rubionic-workspace workers** that require:

- **Privileged container access** for Nix daemon socket communication
- **Direct host resource mounting** (NVMe drives, cache directories, sockets)
- **Single-maintainer projects** (not highly multi-tenant environments)
- **Bare metal performance** with local NVMe cache optimization

### Target Architecture: Cached-Privileged-Kubernetes

The `cached-privileged-kubernetes` container mode provides:

```yaml
securityContext:
  privileged: true                    # Full privileged access
  allowPrivilegeEscalation: true      # Can escalate privileges  
  readOnlyRootFilesystem: false       # Can write to filesystem
  runAsNonRoot: false                 # Can run as root

volumeMounts:
  - name: nix-store                   # Direct Nix store access  
    mountPath: /nix/store-host
  - name: nix-daemon-socket           # Nix daemon socket access
    mountPath: /nix/var/nix/daemon-socket-host
  - name: docker-cache                # Docker image cache persistence
    mountPath: /var/lib/docker
```

### Critical Architecture: Workflow-Managed Docker

**Key Understanding**: DeskRun does NOT provide a Docker daemon. Instead:

1. **Workflows install Docker**: The GitHub Actions workflow installs and starts Docker daemon inside the privileged container
2. **Cache volume persists images**: `/var/lib/docker` is mounted to cache Docker images/layers between runs
3. **Privileged mode enables daemon**: `privileged: true` allows workflow to start Docker daemon with full system access
4. **Performance optimization**: Cached Docker images dramatically speed up subsequent builds

**Example Workflow Pattern**:
```yaml
jobs:
  build:
    runs-on: rubionic-workspace-1
    steps:
      - name: Install Docker
        run: |
          # Install Docker daemon in privileged container
          curl -fsSL https://get.docker.com | sh
          systemctl start docker
      
      - name: Build with cached images
        run: |
          # Docker images are cached in /var/lib/docker volume
          docker build . # Fast due to cached layers
```

**Why This Architecture**:
- ✅ **Maximum flexibility**: Workflow controls Docker version and configuration
- ✅ **Cache optimization**: Docker images persist between workflow runs
- ✅ **No resource overhead**: No always-running Docker daemon in runner
- ✅ **Security isolation**: Docker daemon only exists during workflow execution

### Design Philosophy

#### 1. **Bare Metal Optimization**
- **NVMe Cache Integration**: Configurable cache paths for mounting high-speed NVMe storage
- **Direct Hardware Access**: Privileged containers can access host hardware optimally
- **Minimal Overhead**: No nested containerization (DinD) - direct execution

#### 2. **Rubionic-Workspace Compatibility**
- **Nix Daemon Access**: Solves `Host daemon socket not found` errors
- **Store Mounting**: Direct `/nix/store` access for package management
- **Development Workflow Support**: Full filesystem access for build artifacts

#### 3. **Configuration Agnostic**
- **Flexible Cache Paths**: Not hardcoded - configurable via `CachePaths` array
- **Adaptable Mounting**: Support for `hostPath`, `emptyDir`, and custom volume types
- **Multi-Purpose**: While optimized for rubionic-workspace, supports other privileged workloads

#### 4. **Single-Maintainer Focus**
- **Simplified Security Model**: Privileged access acceptable for trusted single-maintainer projects
- **Performance Over Isolation**: Prioritizes speed and functionality over multi-tenant security
- **Direct Control**: Full host access when needed for development workflows

## Architecture Components

### Container Mode: `cached-privileged-kubernetes`

**Runner Container Features:**
- `privileged: true` - Full system access
- `ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER: false` - Direct execution
- `/home/runner/k8s-novolume/index.js` - Optimized hook system
- Direct volume mounts for cache, sockets, and storage

**Host Integration:**
- **Nix Infrastructure**: `/nix/store` and daemon socket mounting  
- **Docker Image Cache**: `/var/lib/docker` persistence for workflow-installed Docker
- **Cache Optimization**: Configurable NVMe cache paths for builds
- **Development Tools**: Full filesystem access for builds

### Cache Configuration

```go
type CachePath struct {
    Source string  // Host path (empty = emptyDir)
    Target string  // Container mount point
}

// Example: NVMe cache optimization
cachePaths := []CachePath{
    {Source: "/nvme/nix-store", Target: "/nix/store"},
    {Source: "/nvme/cache", Target: "/cache"}, 
    {Source: "", Target: "/tmp/build"},  // emptyDir for temporary builds
}
```

### Security Model

**Single-Maintainer Trust Assumption:**
- All projects maintained by same trusted maintainer
- Privileged access acceptable for development workflows
- Performance and functionality prioritized over isolation
- No untrusted code execution expected

**Host Protection:**
- Privileged containers operate within Kubernetes security boundaries
- Resource limits and namespace isolation still apply
- Network policies and RBAC provide additional controls

## Use Cases

### Primary: Rubionic-Workspace Development

**Workflow Requirements:**
- Nix package manager with daemon access
- Docker daemon installation and image caching 
- Large build artifacts requiring NVMe cache
- Development tool chains needing filesystem access
- Container operations with workflow-managed Docker

**Performance Optimizations:**
- NVMe-backed Nix store for fast package access
- Persistent Docker image cache for faster builds
- Local cache mounting for build artifacts
- Direct hardware access for optimal I/O
- Minimal containerization overhead

### Secondary: High-Performance CI/CD

**Suitable for:**
- Single-maintainer projects requiring privileged access
- Build workflows needing host resource access
- Development environments requiring full system access
- Performance-critical CI/CD pipelines

**Not Suitable for:**
- Multi-tenant environments with untrusted code
- Security-focused deployments requiring isolation
- Shared infrastructure with multiple maintainers
- Compliance environments requiring privilege restrictions

## Deployment Considerations

### Bare Metal Infrastructure

**Recommended Setup:**
- Kubernetes cluster on bare metal nodes
- NVMe drives mounted at consistent paths
- Docker runtime with privileged container support
- Network storage for shared artifacts (optional)

**NVMe Cache Strategy:**
```yaml
# Node preparation
/nvme/cache/nix-store     # Nix package cache
/nvme/cache/builds        # Build artifact cache  
/nvme/cache/docker        # Docker image/layer cache (workflow-installed)
/nvme/cache/cargo         # Rust/Cargo cache
/nvme/cache/npm           # Node.js cache
```

### Configuration Examples

**Basic Rubionic-Workspace Setup:**
```yaml
containerMode: cached-privileged-kubernetes
cachePaths:
  - source: "/nvme/nix-store"
    target: "/nix/store-host"
  - source: "/nvme/nix-daemon"  
    target: "/nix/var/nix/daemon-socket-host"
  - source: "/nvme/docker-cache"
    target: "/var/lib/docker"    # Docker image cache for workflow-installed Docker
  - source: "/nvme/cargo-cache"
    target: "/home/runner/.cargo"
  - source: ""
    target: "/tmp/builds"
```

## Security & Trust Model

### Assumptions

1. **Single Maintainer**: All code and workflows controlled by trusted maintainer
2. **Development Focus**: Primarily development/build workloads, not production services
3. **Bare Metal Control**: Full control over underlying infrastructure
4. **Performance Priority**: Speed and functionality over isolation

### Mitigations

1. **Kubernetes Boundaries**: Privileged containers still within K8s security model
2. **Resource Limits**: CPU, memory, and storage quotas prevent resource exhaustion
3. **Network Policies**: Control network access even for privileged containers
4. **Monitoring**: Comprehensive logging and monitoring of privileged operations

### Risk Acceptance

- **Privileged Escalation**: Accepted for development workflow requirements
- **Host Access**: Required for Nix daemon and cache optimization
- **Container Breakout**: Risk mitigated by single-maintainer trust model
- **Resource Abuse**: Controlled through Kubernetes resource management

## Performance Characteristics

### NVMe Cache Benefits

- **Nix Store Access**: 10-100x faster package resolution vs network
- **Build Artifacts**: Persistent cache across workflow runs
- **Docker Images**: Local registry cache for base images
- **Development Tools**: Cached toolchains and dependencies

### Benchmark Targets

- **Cold Build**: < 2 minutes for typical rubionic-workspace setup
- **Warm Build**: < 30 seconds with full NVMe cache
- **Package Resolution**: < 5 seconds for cached Nix packages
- **Container Startup**: < 10 seconds including cache mounts

## Future Considerations

### Potential Enhancements

1. **Cache Warming**: Pre-populate NVMe caches during node setup
2. **Multi-Cache Strategy**: Support for tiered cache (NVMe + network)
3. **Monitoring Integration**: Performance metrics for cache hit rates
4. **Auto-Scaling**: Dynamic cache allocation based on usage patterns

### Architectural Evolution

- **Hybrid Security**: Optional non-privileged mode for less trusted workloads
- **Multi-Tenant Support**: Enhanced isolation for shared infrastructure
- **Cloud Integration**: Adapt optimizations for cloud-based NVMe storage
- **Container Registry**: Built-in registry for cached container images

---

## Development Workflows

### ACCEPT_DIFF: Regenerating Expected Test Files

When templates change (overlays, schema, or base templates), the expected test output files need to be regenerated. Use the `ACCEPT_DIFF=1` environment variable to automatically update expected files from actual test output.

**When to use:**
- After modifying templates in `pkg/templates/templates/`
- When adding new container modes or overlay configurations
- When test output intentionally changes due to feature updates

**Usage:**
```bash
# Regenerate expected files for pkg/templates tests
ACCEPT_DIFF=1 go test ./pkg/templates/...

# Regenerate expected files for internal/runner tests  
ACCEPT_DIFF=1 go test ./internal/runner/...

# Regenerate all expected files
ACCEPT_DIFF=1 go test ./...
```

**How it works:**
1. Tests render templates with various configurations
2. When `ACCEPT_DIFF=1` is set, actual output overwrites expected files
3. Expected files are stored in `testdata/expected/` directories:
   - `pkg/templates/testdata/expected/` - processor tests
   - `internal/runner/template_spec/testdata/expected/` - runner tests

**After regenerating:**
1. Review the diffs carefully with `git diff`
2. Verify changes are intentional and correct
3. Run tests again without `ACCEPT_DIFF` to confirm they pass
4. Commit the updated expected files with the template changes

**Example workflow:**
```bash
# 1. Make template changes
vim pkg/templates/templates/overlays/container-mode-privileged.yaml

# 2. Regenerate expected files
ACCEPT_DIFF=1 go test ./...

# 3. Review changes
git diff

# 4. Verify tests pass
go test ./...

# 5. Commit together
git add -A && git commit -m "feat: update privileged mode template"
```

---

## Summary

**DeskRun prioritizes performance and functionality over isolation for single-maintainer development environments.** The cached-privileged-kubernetes mode provides optimal bare metal performance with NVMe cache integration, specifically designed for rubionic-workspace workflows requiring Nix daemon access and high-speed build caching.

The architecture accepts security trade-offs in exchange for development velocity and infrastructure efficiency, making it ideal for trusted single-maintainer projects but unsuitable for multi-tenant or security-focused deployments.