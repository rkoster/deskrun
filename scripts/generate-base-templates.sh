#!/bin/bash
# Generate base templates from Helm for each container mode
# This script creates pre-rendered templates for kubernetes, dind, and privileged modes
# that are selected at runtime based on the container mode configuration.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
UPSTREAM_DIR="$PROJECT_ROOT/upstream"
OUTPUT_DIR="$PROJECT_ROOT/pkg/templates/templates/scale-set/bases"

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

echo "Generating base templates from Helm..."

# Common values for all modes
COMMON_VALUES=(
    --namespace arc-systems
    --set githubConfigUrl=https://github.com/example/repo
    --set githubConfigSecret.github_token=placeholder
    --set controllerServiceAccount.name=arc-gha-rs-controller
    --set controllerServiceAccount.namespace=arc-systems
)

# Generate Kubernetes mode template
echo "  Generating kubernetes mode template..."
helm template arc-runner "$UPSTREAM_DIR/gha-runner-scale-set" \
    "${COMMON_VALUES[@]}" \
    --set containerMode.type=kubernetes \
    --set 'containerMode.kubernetesModeWorkVolumeClaim.accessModes={ReadWriteOnce}' \
    --set containerMode.kubernetesModeWorkVolumeClaim.storageClassName=standard \
    --set containerMode.kubernetesModeWorkVolumeClaim.resources.requests.storage=1Gi \
    | sed 's/^# Source:/#! Source:/g' \
    > "$OUTPUT_DIR/kubernetes.yaml"
echo "    -> $OUTPUT_DIR/kubernetes.yaml"

# Generate DinD mode template
echo "  Generating dind mode template..."
helm template arc-runner "$UPSTREAM_DIR/gha-runner-scale-set" \
    "${COMMON_VALUES[@]}" \
    --set containerMode.type=dind \
    | sed 's/^# Source:/#! Source:/g' \
    > "$OUTPUT_DIR/dind.yaml"
echo "    -> $OUTPUT_DIR/dind.yaml"

# Generate Privileged mode template (kubernetes-novolume)
# This is used for cached-privileged-kubernetes mode
echo "  Generating privileged mode template..."
helm template arc-runner "$UPSTREAM_DIR/gha-runner-scale-set" \
    "${COMMON_VALUES[@]}" \
    --set containerMode.type=kubernetes \
    --set 'containerMode.kubernetesModeWorkVolumeClaim.accessModes={ReadWriteOnce}' \
    --set containerMode.kubernetesModeWorkVolumeClaim.storageClassName=standard \
    --set containerMode.kubernetesModeWorkVolumeClaim.resources.requests.storage=1Gi \
    | sed 's/^# Source:/#! Source:/g' \
    > "$OUTPUT_DIR/privileged.yaml"
echo "    -> $OUTPUT_DIR/privileged.yaml"

echo ""
echo "Base templates generated successfully!"
echo "  - kubernetes.yaml: Standard Kubernetes mode with job containers"
echo "  - dind.yaml: Docker-in-Docker mode"
echo "  - privileged.yaml: Privileged Kubernetes mode (cached-privileged-kubernetes)"
