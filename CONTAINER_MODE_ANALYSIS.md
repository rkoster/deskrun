# Container Mode Analysis: Privileged-Kubernetes vs Current Implementation

## Executive Summary

After analyzing the nixpkgs `privileged-kubernetes` implementation and ARC's official documentation, I've identified a superior approach to the current `cached-privileged-kubernetes` mode in deskrun. The nixpkgs implementation uses a **hook extension pattern with ConfigMaps** to inject security context and cgroup mounts into job pods created by the runner, combined with `kubernetes-novolume` mode. This approach is more maintainable, portable, and follows GitHub's recommended advanced configuration patterns.

## Current Implementation Analysis

### Current `cached-privileged-kubernetes` Mode

The current deskrun implementation directly modifies the runner pod template to add:
- Privileged security context
- All required capabilities
- Custom volume mounts for Docker cache
- Manual host path mounts

**Problems:**
1. ✗ Runner pod itself is privileged (unnecessary security risk)
2. ✗ Host path mounts are not portable across clusters
3. ✗ Cache path handling is hardcoded and instance-specific
4. ✗ Job pods inherit runner pod settings (still not optimal for job isolation)
5. ✗ No support for pod lifecycle hooks to manage cache between pods
6. ✗ Not following GitHub's official advanced configuration patterns

**Code Location:** `internal/runner/runner.go:365-432`

## Nixpkgs `privileged-kubernetes` Implementation Analysis

### Architecture

Uses a **three-layer approach**:

1. **Runner Pod (Non-Privileged)**
   - Runs as non-root unless necessary
   - Mounts hook extension ConfigMap
   - Sets `ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE=/etc/hooks/content`
   - Sets `ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER=false`
   - Uses work volume (ephemeral or emptyDir)

2. **Hook Extension ConfigMap**
   - Contains PodSpec patch as YAML
   - Applied by `runner-container-hooks` when job pod is created
   - Injects: privileged security context, capabilities, cgroup mounts
   - Target: Job container (using `$job` placeholder)
   - Separate ConfigMap per installation (for multi-instance support)

3. **Job Pods (Privileged via Hooks)**
   - Created by runner's `runner-container-hooks`
   - Automatically patched with security context
   - Mounted with cgroup/proc/dev for nested Docker
   - Cleaned up automatically when job completes

### Key Features

**Hook Extension ConfigMap Structure:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: privileged-hook-extension-<installation-name>
  namespace: arc-runners
data:
  content: |
    spec:
      securityContext:
        runAsUser: 0
        runAsGroup: 0
        fsGroup: 0
      containers:
        - name: "$job"
          securityContext:
            privileged: true
            capabilities:
              add:
                - SYS_ADMIN
                - NET_ADMIN
                - [... 8 more capabilities ...]
          volumeMounts:
            - name: cgroup
              mountPath: /sys/fs/cgroup
              readOnly: false
              mountPropagation: Bidirectional
            - name: proc
              mountPath: /proc
            - name: dev
              mountPath: /dev
      volumes:
        - name: cgroup
          hostPath:
            path: /sys/fs/cgroup
            type: Directory
        - name: proc
          hostPath:
            path: /proc
            type: Directory
        - name: dev
          hostPath:
            path: /dev
            type: Directory
```

**Runner Pod Template:**
```yaml
template:
  spec:
    containers:
      - name: runner
        env:
          - name: ACTIONS_RUNNER_CONTAINER_HOOKS
            value: /home/runner/k8s/index.js
          - name: ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE
            value: "/etc/hooks/content"
          - name: ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER
            value: "false"
        volumeMounts:
          - name: work
            mountPath: /home/runner/_work
          - name: privileged-hook-extension
            mountPath: /etc/hooks
            readOnly: true
    volumes:
      - name: work
        ephemeral:  # or emptyDir
          volumeClaimTemplate:
            spec:
              storageClassName: "standard"
              resources:
                requests:
                  storage: <size>
      - name: privileged-hook-extension
        configMap:
          name: privileged-hook-extension-<name>
```

**Caching Strategy:**
- Optional ephemeral persistent volume for work directory (docker cache)
- Job pods created fresh (or with lifecycle hooks for novolume mode)
- Cache lifetime tied to ephemeral volume lifecycle
- Supports optional `dockerCacheSize` configuration

### Advantages of Nixpkgs Approach

1. ✓ **Principle of Least Privilege**: Runner pod is not privileged
2. ✓ **Job Pod Isolation**: Privileged only where needed (job container)
3. ✓ **Configuration-Driven**: ConfigMap allows easy updates without pod recreation
4. ✓ **Portable**: Uses hook extensions (ARC standard mechanism)
5. ✓ **Multi-Instance Support**: Per-installation ConfigMaps
6. ✓ **Follows ARC Guidelines**: Aligns with GitHub's advanced configuration patterns
7. ✓ **Flexible Caching**: Optional ephemeral volumes for cache
8. ✓ **Cleaner Separation**: Hooks manage job pod config separately from runner pod
9. ✓ **Easier to Update**: Security context changes only require ConfigMap update

## GitHub ARC Documentation Insights

### Hook Extension Pattern (Official Support)

ARC v0.4.0+ explicitly supports hook extensions via `ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE`:

> "Hook extensions allow you to specify a YAML file that is used to update the [PodSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#podspec-v1-core) of the pod created by runner-container-hooks."

**Two storage options:**
1. **Custom runner image** - Embed YAML in image
2. **ConfigMap (Recommended)** - Mount ConfigMap and reference path

**Container Targeting:**
- Use `$job` as container name to target job container
- Fields are merged/appended intelligently
- Supports: `env`, `volumeMounts`, `ports`, `securityContext`, etc.

### `kubernetes-novolume` Mode

From GitHub docs:

> "When using `kubernetes-novolume` mode, the container must run as `root` to support lifecycle hook operations."

**Benefits:**
- No persistent volumes required
- Uses container lifecycle hooks to restore/export job filesystems
- Better for local storage and portability
- Ideal for clusters without RWX volume support

## Proposed Improved Mode: `privileged-kubernetes-hooks`

### Design Specification

**Name**: `privileged-kubernetes-hooks` or `privileged-novolume`

**Base**: Combines `kubernetes-novolume` mode with privileged hook extension

**Architecture**:

```
Runner Pod (kubernetes-novolume mode)
├── Non-privileged container (unless hooks require root)
├── Runner container hooks: enabled (default)
├── Container hook template: /etc/hooks/content
└── ConfigMap mount: privileged-hook-extension -> /etc/hooks

Hook Extension ConfigMap
├── Applies to job container only ($job)
├── Privileged security context
├── SYS_ADMIN, NET_ADMIN, SYS_PTRACE + 8 more capabilities
├── cgroup, proc, dev mounts from host
└── hostPath volumes for above

Job Pods (Created by hooks)
├── Receives hook extension patches
├── Privileged security context in job container
├── Host cgroup/proc/dev mounts for nested Docker
└── Ephemeral storage (or cache-enabled)
```

### Configuration Options

```go
type ContainerModeConfig struct {
    Type                    string           // "privileged-kubernetes-hooks"
    CachePaths              []CachePath      // Host path mounts for caching
    EphemeralVolumePaths    []string         // Paths to mount from ephemeral volume
    DockerCacheSize         string           // Optional: "20Gi", "50Gi" etc
    BuildCacheSize          string           // Optional: "10Gi", "20Gi" etc
    RunAsRoot               bool             // Runner container as root (default: true)
}
```

### Implementation Plan

#### Phase 1: Hook Extension ConfigMap Generator

Create function to generate ConfigMap:

```go
func generatePrivilegedHookExtension(installation *types.RunnerInstallation, instanceNum int) string {
    // Generate ConfigMap YAML with:
    // - metadata: name, namespace, labels
    // - data.content: PodSpec patch with privileges, capabilities, host mounts
    // - Support for optional cache path bind mounts via hooks
    return configMapYAML
}
```

#### Phase 2: Runner Pod Template Update

Modify template generation:

```go
func generateKuberneteHooksTemplate(installation *types.RunnerInstallation, instanceNum int) string {
    // Generate template with:
    // - containerMode: remove (using hooks pattern)
    // - ACTIONS_RUNNER_CONTAINER_HOOKS: /home/runner/k8s/index.js
    // - ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE: /etc/hooks/content
    // - ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER: false (if needed)
    // - volumeMounts: privileged-hook-extension ConfigMap
    // - volumes: privileged-hook-extension ConfigMap reference
    return templateYAML
}
```

#### Phase 3: Helm Values Generation

Update `generateHelmValues()`:

```go
case types.ContainerModePrivilegedKubernetesHooks:
    containerModeConfig = m.generatePrivilegedKubernetesHooks(installation, instanceNum)
    // This would:
    // 1. Create hook extension ConfigMap
    // 2. Generate runner template with hook reference
    // 3. Return template YAML
```

### Advantages Over Current Mode

| Aspect | Current | Proposed |
|--------|---------|----------|
| Runner Security | Privileged | Non-privileged |
| Job Container | Inherits runner | Explicitly privileged via hook |
| Configuration | Pod template | ConfigMap (hot-updatable) |
| Portability | Host paths hardcoded | Standard Kubernetes patterns |
| Cache Support | Hardcoded volumes | Optional, configurable |
| Isolation | Limited | Better (hook-based) |
| Standards Compliance | Custom | ARC official pattern |
| Update Mechanism | Pod restart | ConfigMap patch |

### Cache Management Strategy

**Option A: Ephemeral Volumes (Like Nixpkgs)**
- Optional `dockerCacheSize` enables ephemeral PVC
- Survives runner pod restart but recreated when scale-down occurs
- Good for: Docker layer cache, build artifacts
- ConfigMap hook can bind mount to specific paths in job

**Option B: Hook-Based Lifecycle**
- Use `postJobCmd` hook to export cache before pod deletion
- Use `initJobCmd` hook to restore cache when pod created
- No persistent storage needed
- Better for: Multi-cluster, edge scenarios

**Option C: Hybrid (Recommended)**
- Optional ephemeral volume for work directory
- Hook extensions inject cache mount paths
- Job pods mount cache from ephemeral volume
- Supports `dockerCacheSize` and `buildCacheSize` configuration

### Migration Path

1. Add `privileged-kubernetes-hooks` as new container mode
2. Keep `cached-privileged-kubernetes` for backwards compatibility (mark as deprecated)
3. Document migration steps
4. Provide helper to convert old config to new mode
5. Eventually deprecate old mode in v2.0

## Implementation Considerations

### What Changes Are Needed

1. **runner.go**: Add new mode generation functions
2. **types.go**: Add `ContainerModePrivilegedKubernetesHooks` enum value
3. **Helm values**: Template generation for new mode
4. **ConfigMap generation**: Create hook extension ConfigMap
5. **Documentation**: Explain new mode and migration

### What Doesn't Need Changes

- ARC controller (handles it automatically via hooks)
- Kubernetes cluster (standard patterns only)
- GitHub API integration
- Authentication mechanisms

### Compatibility

- **Kubernetes versions**: Any version with hook extension support (v1.19+)
- **ARC versions**: v0.4.0+ (hook extension support)
- **Container runtime**: Any (hooks are runtime-agnostic)

## Risks and Mitigation

| Risk | Severity | Mitigation |
|------|----------|-----------|
| Job pod PrivEsc via hooks | Medium | Limit hook extension scope, validate ConfigMap content |
| cgroup mount escape | Low | Use read-only where possible, monitor kernel patches |
| Missing capabilities | Low | Document all required capabilities with rationale |
| Cache contention | Low | Use ephemeral volumes (automatic isolation) |
| Hook failure | Medium | Add fallback template, robust error handling |

## Recommendations

1. **Implement `privileged-kubernetes-hooks` mode** as the new standard
2. **Use ephemeral volumes** for cache (better than host paths)
3. **Follow ARC's hook extension pattern** (official, supported, portable)
4. **Deprecate old mode** with clear migration documentation
5. **Add validation** for ConfigMap content before applying
6. **Support both ephemeral and lifecycle-hook** cache strategies

## Reference Implementation Comparison

### Nixpkgs
- ✓ Uses hook extensions
- ✓ Supports caching via ephemeral volumes
- ✓ Instance-specific ConfigMaps
- ✓ Optional cache sizes
- ~ Bash-based (less maintainable)

### Proposed Deskrun
- ✓ Type-safe Go implementation
- ✓ Follows ARC best practices
- ✓ Reusable for other modes
- ✓ Clear separation of concerns
- ✓ Better error handling
- ✓ Integration with existing types/config system

## Testing Strategy

1. **Unit tests**: ConfigMap generation, template validation
2. **Integration tests**: Runner creation, job pod creation with hooks
3. **End-to-end**: Full workflow with Docker build in job
4. **Backwards compatibility**: Existing `cached-privileged-kubernetes` mode still works
5. **Performance**: Cache persistence and performance metrics

## Success Criteria

- [ ] New mode supports nested Docker containers in job pods
- [ ] ConfigMap updates take effect without runner pod restart
- [ ] Cache (optional) persists across job runs
- [ ] Migration from old mode is documented and easy
- [ ] Security posture is improved (least privilege principle)
- [ ] All tests pass, including backwards compatibility
- [ ] Documentation updated with examples and rationale
