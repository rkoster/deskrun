# deskrun

DeskRun: Unlocking Local Compute for GitHub Actions.

## Overview

`deskrun` is a CLI tool for running GitHub Actions locally using kind (Kubernetes in Docker) clusters. It provides easy management of local GitHub Actions runners with optimized configurations based on lessons learned from production deployments.

## Features

- **Simple CLI Interface**: Easy-to-use commands for managing runner installations
- **Multiple Container Modes**: Support for standard, privileged, and Docker-in-Docker modes
- **Persistent Caching**: Host path volume caching for Nix store, Docker daemon, and other paths
- **Kind Cluster Management**: Automatic cluster creation and management
- **Flexible Authentication**: Support for GitHub Personal Access Tokens (PAT) and GitHub Apps

## Installation

### Prerequisites

- Go 1.20 or later
- Docker
- kubectl
- kind
- helm (v3.x)

### Build from Source

```bash
git clone https://github.com/rkoster/deskrun.git
cd deskrun
make build
sudo make install
```

## Usage

### Adding a Runner Installation

Add a new GitHub Actions runner installation to the kind cluster:

```bash
# Standard runner
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx

# Privileged runner with Nix cache
deskrun add nix-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --cache /nix/store \
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

- **Use case**: Repositories requiring systemd, cgroup access, nested Docker, or Nix builds
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
- **Benefits**: Clean Docker environment, good for OpenCode workspaces

## Cache Paths

For performance-critical paths like `/nix/store`, `/var/lib/docker`, or `/root/.cache`, you can specify cache paths that will be mounted using hostPath volumes:

```bash
deskrun add nix-runner \
  --repository https://github.com/owner/repo \
  --cache /nix/store \
  --cache /root/.cache \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

Cache paths are automatically partitioned per installation at:
```
/tmp/github-runner-cache/{installation-name}/cache-{index}
```

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

## Lessons Learned

This tool incorporates lessons learned from extensive refactoring of a GitHub Actions Runner Controller setup:

1. **Container Mode Issues**: Avoid incompatible container mode settings that cause runners to cycle
2. **Privileged Requirements**: Properly configure capabilities and security contexts for nested Docker
3. **Cache Strategy**: Use host path volumes for performance-critical paths
4. **Authentication**: Support both PAT and GitHub App authentication methods

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

[Add your license here]

## Contributing

Contributions are welcome! Please open an issue or pull request.
