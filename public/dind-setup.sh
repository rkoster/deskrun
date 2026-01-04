#!/bin/bash
# dind-setup.sh - Docker-in-Docker Setup
# Purpose: Start Docker daemon for container builds within workflows

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[dind-setup]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[dind-setup]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[dind-setup]${NC} $1"
}

log_error() {
    echo -e "${RED}[dind-setup]${NC} $1" >&2
}

fail() {
    log_error "$1"
    exit 1
}

start_dockerd() {
    log_info "Starting Docker-in-Docker setup..."

    if [ -n "$NIX_REMOTE" ]; then
        log_success "NIX_REMOTE is set to: $NIX_REMOTE"
    elif command -v nix >/dev/null 2>&1; then
        log_success "nix command is available"
    else
        fail "nix is not configured - please run nix-setup.sh first"
    fi

    log_info "Installing docker via nix profile..."
    if nix profile install nixpkgs#docker; then
        log_success "docker installed successfully"
    else
        fail "Failed to install docker"
    fi

    log_info "Enabling cgroup delegation for nested containers..."
    if echo '+cpuset +cpu +io +memory +pids' > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null; then
        log_success "cgroup delegation enabled"
    else
        log_warn "Direct cgroup write failed, trying fallback with init.scope..."
        mkdir -p /sys/fs/cgroup/init.scope
        if echo '+cpuset +cpu +io +memory +pids' > /sys/fs/cgroup/init.scope/cgroup.subtree_control 2>/dev/null; then
            for pid in $(cat /sys/fs/cgroup/cgroup.procs); do
                echo "$pid" > /sys/fs/cgroup/init.scope/cgroup.procs 2>/dev/null || true
            done
            log_success "cgroup delegation enabled via init.scope"
        else
            log_error "Failed to enable cgroup delegation - nested containers may not work"
        fi
    fi

    log_info "Creating Docker directories..."
    mkdir -p /var/lib/docker
    mkdir -p /var/run
    mkdir -p /tmp/dind
    mkdir -p /etc/docker
    mkdir -p /var/log
    log_success "Directories created"

    log_info "Creating Docker daemon configuration..."
    cat > /etc/docker/daemon.json <<EOF
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m"
  },
  "storage-driver": "vfs"
}
EOF
    log_success "daemon.json created"

    log_info "Starting dockerd..."
    dockerd \
        --host=unix:///tmp/dind/docker.sock \
        --data-root=/var/lib/docker \
        --log-level=error \
        --insecure-registry=0.0.0.0/0 \
        >/var/log/dockerd.log 2>&1 &
    
    DOCKERD_PID=$!
    log_info "dockerd started with PID: $DOCKERD_PID"

    log_info "Waiting for Docker daemon to be ready (30s timeout)..."
    TIMEOUT=30
    ELAPSED=0
    export DOCKER_HOST=unix:///tmp/dind/docker.sock
    
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if docker info >/dev/null 2>&1; then
            log_success "Docker daemon is ready!"
            return 0
        fi
        sleep 1
        ELAPSED=$((ELAPSED + 1))
    done

    log_error "Docker daemon failed to start within ${TIMEOUT}s"
    log_error "Last 20 lines of dockerd log:"
    tail -n 20 /var/log/dockerd.log >&2
    return 1
}

log_info "Attempting to start Docker daemon..."
if start_dockerd; then
    log_success "Docker daemon started successfully on first attempt"
    RETRY_NEEDED=0
else
    log_warn "First attempt failed, implementing retry logic..."
    RETRY_NEEDED=1
fi

if [ $RETRY_NEEDED -eq 1 ]; then
    log_info "Cleaning up for retry..."
    
    log_info "Killing dockerd processes..."
    pkill -9 dockerd 2>/dev/null || true
    sleep 2
    
    log_info "Clearing /var/lib/docker..."
    rm -rf /var/lib/docker/*
    
    log_info "Retrying dockerd start..."
    if start_dockerd; then
        log_success "Docker daemon started successfully on retry"
    else
        fail "Docker daemon failed to start after retry - check /var/log/dockerd.log"
    fi
fi

if [ -n "$GITHUB_ENV" ]; then
    log_info "Exporting environment variables to GITHUB_ENV..."
    {
        echo "DOCKER_HOST=unix:///tmp/dind/docker.sock"
        echo "DOCKERD_PID=$DOCKERD_PID"
    } >> "$GITHUB_ENV"
    log_success "Environment variables exported to GITHUB_ENV"
else
    log_warn "GITHUB_ENV not set - environment variables not persisted for future steps"
fi

log_success "✅ Docker-in-Docker setup complete!"
log_info "Docker socket: unix:///tmp/dind/docker.sock"
log_info "dockerd PID: $DOCKERD_PID"
