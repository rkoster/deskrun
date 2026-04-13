# deskrun

DeskRun: Unlocking Local Compute for GitHub Actions.

## Overview

`deskrun` is a CLI tool for running GitHub Actions locally using kind (Kubernetes in Docker) clusters. It provides easy management of local GitHub Actions runners with optimized configurations based on lessons learned from production deployments.

## Features

- **Simple CLI Interface**: Easy-to-use commands for managing runner installations
- **Multiple Container Modes**: Support for standard, privileged, and Docker-in-Docker modes
- **Persistent Caching**: Host path volume caching for Docker daemon and other paths
- **Kind Cluster Management**: Automatic cluster creation and management
- **Flexible Authentication**: Support for GitHub Personal Access Tokens (PAT) and GitHub Apps

## Installation

### Prerequisites

- Docker

### Using Nix Flakes (Recommended)

The official way to install deskrun is via Nix flakes:

```bash
# Run directly without installing
nix run github:rkoster/deskrun -- --help

# Install to your profile
nix profile install github:rkoster/deskrun

# Or add to your NixOS/home-manager configuration
{
  inputs.deskrun.url = "github:rkoster/deskrun";
  # ...
}
```

### Build from Source

If you prefer to build from source:

```bash
git clone https://github.com/rkoster/deskrun.git
cd deskrun

# With Nix (recommended)
nix build

# Or with Go (requires Go 1.24 or later)
make build
sudo make install
```

## Usage

### Job Routing with Deskrun

Unlike traditional self-hosted runners that use labels (e.g., `runs-on: [self-hosted]`), ARC ephemeral runners use **scale set names** for job routing. This is GitHub's officially supported method for ARC.

To route jobs to deskrun runners, use the scale set name in your workflow:

```yaml
jobs:
  build:
    runs-on: my-runner  # Use the runner's scale set name
    steps:
      - uses: actions/checkout@v4
      - run: ./build.sh
```

**Why not labels?** GitHub explicitly states that ARC runners cannot use additional labels for targeting. The scale set name is used as a "single label" for the `runs-on` target. This is simpler and more explicit than traditional label-based routing.

### Adding a Runner Installation

Add a new GitHub Actions runner installation to the kind cluster:

```bash
# Standard runner
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx

# Privileged runner with Docker cache
deskrun add docker-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx

# Multiple instances for cache isolation (creates runner-1, runner-2, runner-3)
# Each instance automatically gets min=1, max=1 for proper cache isolation
deskrun add multi-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --instances 3 \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx

# Docker-in-Docker runner
deskrun add dind-runner \
  --repository https://github.com/owner/repo \
  --mode dind \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

### Listing Installations

List all configured runner installations:

```bash
deskrun list
```

### Checking Status

Check the status of runner installations:

```bash
# All runners
deskrun status

# Specific runner
deskrun status my-runner
```

### Removing a Runner Installation

Remove a runner installation:

```bash
deskrun remove my-runner
```

## Container Modes

### Standard Mode (`kubernetes`)

- **Use case**: Simple repositories without nested containerization needs
- **Configuration**: `--mode kubernetes`
- **Benefits**: Lightweight, reliable, good for basic CI/CD

### Privileged Mode (`cached-privileged-kubernetes`)

- **Use case**: Repositories requiring systemd, cgroup access, or nested Docker
- **Configuration**: `--mode cached-privileged-kubernetes`
- **Capabilities**: SYS_ADMIN, NET_ADMIN, SYS_PTRACE, SYS_CHROOT, and more
- **Features**:
  - Runs as root (UID 0)
  - Privileged container
  - Full system access
  - SYSTEMD_IGNORE_CHROOT=1 environment variable

### DinD Mode (`dind`)

- **Use case**: Full Docker access via TCP socket
- **Configuration**: `--mode dind`
- **Benefits**: Clean Docker environment with isolated daemon

## Cache Paths

For performance-critical paths like `/var/lib/docker` or `/root/.cache`, you can specify cache paths that will be mounted using hostPath volumes:

```bash
deskrun add docker-runner \
  --repository https://github.com/owner/repo \
  --cache /var/lib/docker \
  --cache /root/.cache \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

You can also specify custom source and target paths using the `src:target` notation:

```bash
deskrun add custom-cache-runner \
  --repository https://github.com/owner/repo \
  --cache /host/cache/npm:/root/.npm \
  --cache /host/cache/docker:/var/lib/docker \
  --cache /root/.cache \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

Cache path formats:
- **Target only**: `--cache /target/path` - Auto-generates host path
- **Source and target**: `--cache /host/path:/container/path` - Use custom host path

Cache paths are automatically partitioned per installation when auto-generated:
```
/tmp/github-runner-cache/{installation-name}/cache-{index}
```

When using custom host paths with `src:target` notation, the specified host path is used directly.

## Multiple Instances

For better cache isolation and deterministic cache affinity, you can create multiple separate runner scale set instances:

```bash
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --instances 3 \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

This creates 3 separate AutoscalingRunnerSets:
- `my-runner-1`
- `my-runner-2`
- `my-runner-3`

Each instance:
- Has dedicated cache partitions (no coordination overhead)
- Runs exactly 1 runner (min=1, max=1) for deterministic behavior
- Provides deterministic cache behavior
- Can be targeted independently by workflows

### Workflow Selection

Use modulo-based routing for deterministic distribution:

```yaml
jobs:
  build:
    runs-on: my-runner-${{ github.event.issue.number % 3 + 1 }}
    steps:
      - run: echo "Running on deterministic instance"
```

**Benefits:**
- Simplified cache management (no locking required)
- Better cache isolation and predictable performance
- Issue-based cache affinity for related workflows
- Improved cache hit rates for follow-up work

## Authentication

### Personal Access Token (PAT)

Create a PAT with `repo` and `workflow` scopes:

```bash
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

### GitHub App

Create a GitHub App and use its private key:

```bash
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --auth-type github-app \
  --auth-value "$(cat private-key.pem)"
```

## Configuration

Configuration is stored in `~/.deskrun/config.json`:

```json
{
  "cluster_name": "deskrun",
  "installations": {
    "my-runner": {
      "Name": "my-runner",
      "Repository": "https://github.com/owner/repo",
      "ContainerMode": "kubernetes",
      "MinRunners": 1,
      "MaxRunners": 5,
      "CachePaths": [],
      "AuthType": "pat",
      "AuthValue": "ghp_xxxxxxxxxxxxx"
    }
  }
}
```

## Architecture

`deskrun` uses the following components:

1. **kind**: Creates a local Kubernetes cluster
2. **Actions Runner Controller (ARC)**: Manages GitHub Actions runners in Kubernetes
3. **AutoscalingRunnerSet**: Kubernetes custom resource for runner scale sets

The tool automatically:
- Creates a kind cluster if it doesn't exist
- Installs the ARC controller and CRDs (Custom Resource Definitions) on first runner installation using Helm
- Deploys each runner scale set using Helm with optimized configurations
- Manages authentication via Helm chart values

**Note**: The first time you add a runner, `deskrun` will automatically install the GitHub Actions Runner Controller using Helm. This may take a minute or two. Each runner is then deployed as a separate Helm release.

## Remote Cluster Hosts (Incus)

For running deskrun on remote infrastructure instead of your local machine, you can provision NixOS containers on Incus hosts with the `cluster-host` command.

### What is a Cluster Host?

A cluster host is a NixOS container running on Incus, pre-configured with:
- Docker daemon
- Kind (Kubernetes in Docker)
- kubectl
- deskrun CLI (via Nix flakes)

This allows you to run GitHub Actions runners on dedicated infrastructure (like a local server or cloud VM running IncusOS) without impacting your local development environment.

### Prerequisites

- An Incus installation (local or remote)
- Access configured via `incus remote` (use `incus remote list` to see available remotes)

### Creating a Cluster Host

```bash
# Create with auto-generated name
deskrun cluster-host create

# Create with custom name and disk size
deskrun cluster-host create --name my-host --disk 300GiB

# Create with specific NixOS image
deskrun cluster-host create --image images:nixos/25.11
```

The creation process:
1. Launches a NixOS container with the specified disk size
2. Configures security.nesting=true (required for Docker-in-container)
3. Installs Docker, Kind, kubectl, and deskrun
4. Runs nixos-rebuild to apply the configuration
5. Saves the cluster host info to your deskrun config

### Using a Cluster Host

Once created, you can access the cluster host and run deskrun commands:

```bash
# Access the container
incus exec my-host -- bash

# Inside the container, use deskrun normally
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /var/lib/docker \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx

deskrun up
```

### Managing Cluster Hosts

```bash
# List all cluster hosts
deskrun cluster-host list

# Re-apply NixOS configuration (useful after deskrun updates)
deskrun cluster-host configure my-host

# Delete a cluster host
deskrun cluster-host delete my-host
```

### Remote Selection

Cluster hosts are created on the current Incus remote. To use a different remote:

```bash
# List available remotes
incus remote list

# Switch to a different remote
incus remote switch my-remote-server

# Now create a cluster host on that remote
deskrun cluster-host create
```

### Container Specifications

- **Image**: NixOS 25.11 container (not VM)
- **Default Disk**: 200GiB (configurable)
- **Security**: Nested containers enabled (required for Docker/Kind)
- **Network**: Outgoing connectivity (no port forwarding needed)
- **Configuration**: Managed via embedded NixOS module

## Troubleshooting

### Runners Not Picking Up Jobs

If jobs remain queued:

1. **Verify runner is online**: `deskrun status my-runner`
2. **Check pod status**: `kubectl get pods -n arc-systems`
3. **Check logs**: `kubectl logs -n arc-systems -l app=my-runner`
4. **Verify you're using scale set name in workflow**: `runs-on: my-runner` not `runs-on: [self-hosted]`

### Cluster Issues

```bash
# Check cluster status
deskrun cluster status

# Recreate cluster if needed
deskrun cluster delete
deskrun cluster create
```

### Permission Errors

For operations requiring elevated permissions (Docker, systemd), use privileged mode:

```bash
deskrun add my-runner \
  --mode cached-privileged-kubernetes \
  --repository https://github.com/owner/repo \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

### Cache Issues

Cache paths are mounted using hostPath volumes. Recommended cache paths:
- `/var/lib/docker` for Docker layer caching
- `/root/.cache` for application caches
- Custom paths like `/tmp/build-cache`

You can use custom host paths for better control:
```bash
# Use custom host paths
deskrun add my-runner \
  --cache /host/persistent/docker:/var/lib/docker \
  --cache /host/persistent/npm:/root/.npm \
  --repository https://github.com/owner/repo \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

### Clean Up

```bash
# Remove specific runner
deskrun remove my-runner

# Clean cache directories
rm -rf /tmp/github-runner-cache/my-runner

# Reset everything
deskrun cluster delete
rm -rf ~/.deskrun
```

## Development

### Build

```bash
make build
```

### Test

```bash
make test
```

### Lint

```bash
make lint
```

### Format

```bash
make fmt
```

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

## Known Limitations

1. **Scale Set Name Routing**: Must use scale set names in workflows, not labels like `[self-hosted]`
2. **Single Cluster**: Manages one kind cluster at a time
3. **Local Development**: Designed for local development, not production deployments

## Contributing

Contributions are welcome! Please open an issue or pull request at https://github.com/rkoster/deskrun.
