# DeskRun CI Bootstrap Scripts

Reusable CI bootstrap scripts for the "deskrun pattern" - enabling persistent nix store caching across CI runs by mounting the host's nix store into workflow containers.

## Overview

These scripts provide a standardized way to bootstrap nix/devbox environments and Docker-in-Docker in GitHub Actions workflows running on deskrun runners. They implement the busybox bootstrap pattern which works reliably with minimal container images.

**What is the "deskrun pattern"?**

The deskrun pattern mounts the host's `/nix/store` and nix daemon socket into workflow containers, providing:
- Persistent nix store across all CI runs
- No repeated downloads of nix packages
- Significantly faster workflow execution
- Consistent environment across all jobs

## Scripts

### `nix-setup.sh` - Core Nix Bootstrap

**Purpose:** Bootstrap nix/devbox environment using host's nix store

**What it does:**
1. **Phase 0:** Copies busybox and SSL certificates before mounting (so essential utilities remain available)
2. **Phase 0.5:** Sets up GitHub workspace directories from deskrun's temporary locations
3. **Phase 1:** Mounts host's nix store and daemon socket, makes nix available
4. **Phase 2:** Configures nix with proper settings and environment variables
5. **Export:** Persists environment variables to `$GITHUB_ENV` for subsequent steps

**Prerequisites:** Nixery container image with busybox

### `install-devbox.sh` - Devbox Installation

**Purpose:** Install devbox via nix profile

**What it does:**
1. Verifies nix is configured
2. Installs devbox using `nix profile install nixpkgs#devbox`
3. Adds devbox to PATH
4. Verifies installation
5. Exports updated PATH to `$GITHUB_ENV`

**Prerequisites:** `nix-setup.sh` must have run first

### `install-docker.sh` - Docker CLI Installation

**Purpose:** Install docker CLI via nix profile for use with host Docker daemon

**What it does:**
1. Verifies nix is configured
2. Installs docker CLI via `nix profile install nixpkgs#docker`
3. Adds docker to PATH
4. Verifies installation
5. Exports updated PATH to `$GITHUB_ENV`

**Prerequisites:** `nix-setup.sh` must have run first

**Note:** This only installs the docker CLI client. For Docker-in-Docker, use `dind-setup.sh` instead.

### `dind-setup.sh` - Docker-in-Docker Setup

**Purpose:** Start Docker daemon for container builds within workflows

**What it does:**
1. Verifies nix is configured
2. Installs docker via `nix profile install nixpkgs#docker`
3. Enables cgroup delegation for nested containers
4. Creates required directories and daemon configuration
5. Starts dockerd with VFS storage driver
6. Waits for daemon to be ready (30s timeout)
7. Implements retry logic (1 retry) on failure
8. Exports `DOCKER_HOST` and `DOCKERD_PID` to `$GITHUB_ENV`

**Prerequisites:** `nix-setup.sh` must have run first

**Note:** For host Docker mode (connecting to host's docker.sock), use `install-docker.sh` instead - it's simpler and faster.

## Prerequisites

### Container Image

Use a Nixery image with required packages:

```yaml
container:
  image: nixery.dev/shell/bash/findutils/coreutils/gnutar/gnugrep/gzip/busybox/cacert
  options: --privileged --init
```

**Required packages:**
- `shell` - Basic shell environment
- `bash` - Bash shell
- `findutils` - File finding utilities
- `coreutils` - Core utilities
- `gnutar` - Tar archiver
- `gnugrep` - Grep utility
- `gzip` - Compression
- `busybox` - Essential utilities (critical for bootstrap)
- `cacert` - SSL certificates

### Container Options

```yaml
options: --privileged --init
```

- `--privileged` - Required for mounting and cgroup operations
- `--init` - Proper signal handling and process management

### Deskrun Runner Configuration

Your deskrun runner must mount the nix store and daemon socket. Use these copy-pastable commands:

#### For Nix/Devbox Only

```bash
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx \
  --cache /nix/store:/nix/store-host \
  --cache /nix/var/nix/daemon-socket:/nix/var/nix/daemon-socket-host:Socket
```

#### For Nix/Devbox + Host Docker

```bash
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx \
  --cache /nix/store:/nix/store-host \
  --cache /nix/var/nix/daemon-socket:/nix/var/nix/daemon-socket-host:Socket \
  --cache /var/run/docker.sock:/var/run/docker.sock:Socket
```

**Note:** The `:Socket` suffix tells deskrun to mount the path as a Unix socket rather than a regular directory.

## Usage Patterns

### Pattern 1: Devbox Only

For workflows that just need nix/devbox:

```yaml
jobs:
  build:
    runs-on: my-deskrun-runner
    container:
      image: nixery.dev/shell/bash/findutils/coreutils/gnutar/gnugrep/gzip/busybox/cacert
      options: --privileged --init
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Nix
        run: .github/scripts/nix-setup.sh
      
      - name: Install Devbox
        run: .github/scripts/install-devbox.sh
      
      - name: Run tests
        run: devbox run test
```

### Pattern 2: Devbox + Docker-in-Docker

For workflows that need to build Docker images:

```yaml
jobs:
  build:
    runs-on: my-deskrun-runner
    container:
      image: nixery.dev/shell/bash/findutils/coreutils/gnutar/gnugrep/gzip/busybox/cacert
      options: --privileged --init
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Nix
        run: .github/scripts/nix-setup.sh
      
      - name: Install Devbox
        run: .github/scripts/install-devbox.sh
      
      - name: Setup Docker (DinD)
        run: .github/scripts/dind-setup.sh
      
      - name: Build image
        run: devbox run -- docker build -t myapp:latest .
      
      - name: Run container tests
        run: devbox run -- docker run myapp:latest npm test
```

### Pattern 3: Devbox + Host Docker

For workflows that use the host's Docker daemon (no DinD needed):

```yaml
jobs:
  build:
    runs-on: my-deskrun-runner
    container:
      image: nixery.dev/shell/bash/findutils/coreutils/gnutar/gnugrep/gzip/busybox/cacert
      options: --privileged --init
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Nix
        run: .github/scripts/nix-setup.sh
      
      - name: Install Devbox
        run: .github/scripts/install-devbox.sh
      
      - name: Install Docker CLI
        run: .github/scripts/install-docker.sh
      
      - name: Build image with host Docker
        run: docker build -t myapp:latest .
      
      - name: Tag and push images
        run: |
          docker tag myapp:latest registry.example.com/myapp:latest
          docker push registry.example.com/myapp:latest
```

**Requirements for host Docker mode:**
1. Ensure your runner mounts `/var/run/docker.sock`
2. Run `install-docker.sh` to install the docker CLI via nix
3. No `dind-setup.sh` needed!

## Vendoring Scripts with vendir

To vendor these scripts into your repository:

### 1. Create `vendir.yml`

```yaml
apiVersion: vendir.carvel.dev/v1alpha1
kind: Config
directories:
  - path: .github/scripts
    contents:
      - path: .
        git:
          url: https://github.com/rkoster/deskrun
          ref: v0.1.0
        includePaths:
          - public/nix-setup.sh
          - public/install-devbox.sh
          - public/install-docker.sh
          - public/dind-setup.sh
        newRootPath: public
```

### 2. Run vendir sync

```bash
vendir sync
```

This will copy the scripts from the deskrun repository's `public/` directory into your `.github/scripts/` directory.

### 3. Commit vendored scripts

```bash
git add .github/scripts/ vendir.yml vendir.lock.yml
git commit -m "Vendor deskrun bootstrap scripts"
```

### 4. Update scripts

When you want to update to a newer version:

```bash
# Update the ref in vendir.yml to new version tag
vendir sync
git add .github/scripts/ vendir.lock.yml
git commit -m "Update deskrun bootstrap scripts to v0.2.0"
```

## Design Decisions

### No Environment Variable Configuration

Scripts have sensible defaults only, with no environment variable overrides. This keeps them simple and predictable.

### Nixery-Based Busybox Bootstrap

The busybox bootstrap pattern is more robust than relying on pre-installed packages because:
- Works with minimal container images
- Ensures essential utilities survive the nix store mount
- Provides consistent tooling across environments

### Separate Scripts

Four focused scripts rather than one monolithic script allows flexibility:
- Not every workflow needs Docker
- Can use host Docker instead of DinD
- Clear separation of concerns
- Easier to debug

### Host Docker Mode Uses `install-docker.sh`

Installing docker CLI via `nix profile install` makes it globally available in PATH, avoiding the need to wrap every docker command in `devbox run`. This is cleaner than adding docker to `devbox.json` where the CLI is only accessible within the devbox environment.

### VFS Storage Driver for DinD

Hardcoded for Docker-in-Docker as it's the safest option (works everywhere). While not the most performant, it's reliable and simple. Storage driver optimization is a future consideration.

## Troubleshooting

### "busybox not found in /nix/store"

**Cause:** Container image doesn't include busybox

**Solution:** Use Nixery image with busybox package:
```yaml
image: nixery.dev/shell/bash/findutils/coreutils/gnutar/gnugrep/gzip/busybox/cacert
```

### "/nix/store-host not found"

**Cause:** Runner doesn't have host nix store mounted

**Solution:** Ensure your deskrun runner configuration includes:
```bash
deskrun add my-runner \
  --repository https://github.com/owner/repo \
  --mode cached-privileged-kubernetes \
  --auth-type pat \
  --auth-value ghp_xxxxxxxxxxxxx
```

### "nix is not configured"

**Cause:** `nix-setup.sh` hasn't run yet or failed

**Solution:** Always run `nix-setup.sh` first, before any other scripts. Check its output for errors.

### "Docker daemon failed to start"

**Cause:** Various issues with DinD setup

**Solutions:**
1. Check `/var/log/dockerd.log` for detailed errors
2. Ensure container has `--privileged` flag
3. Verify cgroup filesystem is available
4. Try host Docker mode instead (simpler)

### "Permission denied" errors

**Cause:** Container not running with required privileges

**Solution:** Ensure container options include:
```yaml
options: --privileged --init
```

### SSL/TLS certificate errors

**Cause:** SSL certificates not properly copied

**Solution:** 
1. Ensure Nixery image includes `cacert` package
2. Check that `/etc/ssl/certs/ca-bundle.crt` exists after nix-setup
3. Verify `NIX_SSL_CERT_FILE` and `SSL_CERT_FILE` are exported

## Environment Variables Exported

### By `nix-setup.sh`

- `NIX_REMOTE=daemon` - Connect to nix daemon
- `NIX_DAEMON_SOCKET_PATH=/nix/var/nix/daemon-socket/socket` - Daemon socket location
- `NIX_SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt` - Nix SSL certificates
- `SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt` - General SSL certificates
- `CURL_CA_BUNDLE=/etc/ssl/certs/ca-bundle.crt` - Curl SSL certificates
- `PATH=$HOME/.nix-profile/bin:/tmp/bootstrap/bin:$PATH` - Updated PATH

### By `install-devbox.sh`

- `PATH=$HOME/.nix-profile/bin:$PATH` - Updated PATH with devbox

### By `install-docker.sh`

- `PATH=$HOME/.nix-profile/bin:$PATH` - Updated PATH with docker CLI

### By `dind-setup.sh`

- `DOCKER_HOST=unix:///tmp/dind/docker.sock` - Docker socket location
- `DOCKERD_PID=<pid>` - Docker daemon process ID

## Script Features

### Colored Log Output

All scripts use colored output for better readability:
- 🔵 Blue - Informational messages
- 🟢 Green - Success messages
- 🟡 Yellow - Warnings
- 🔴 Red - Errors

### Error Handling

All scripts use `set -e` to exit immediately on errors and provide clear error messages with context.

### Idempotency

Scripts check prerequisites before running and fail fast with helpful error messages if dependencies are missing.

### GitHub Actions Integration

All scripts automatically export required environment variables to `$GITHUB_ENV` when available, ensuring configuration persists across workflow steps.

## License

These scripts are part of the deskrun project and licensed under the Apache License, Version 2.0.

## Contributing

Found an issue or have an improvement? Please open an issue or pull request at:
https://github.com/rkoster/deskrun
