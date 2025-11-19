package templates

import (
	"fmt"
	"strings"

	"github.com/rkoster/deskrun/pkg/types"
)

// GenerateRunnerScaleSetManifest generates the Kubernetes manifest for a runner scale set
func GenerateRunnerScaleSetManifest(installation *types.RunnerInstallation, namespace string) (string, error) {
	var templateSpec string

	switch installation.ContainerMode {
	case types.ContainerModeKubernetes:
		templateSpec = generateStandardTemplate(installation)
	case types.ContainerModePrivileged:
		templateSpec = generatePrivilegedTemplate(installation)
	case types.ContainerModeDinD:
		templateSpec = generateDinDTemplate(installation)
	default:
		return "", fmt.Errorf("unsupported container mode: %s", installation.ContainerMode)
	}

	manifest := fmt.Sprintf(`apiVersion: actions.github.com/v1alpha1
kind: AutoscalingRunnerSet
metadata:
  name: %s
  namespace: %s
spec:
  githubConfigUrl: %s
  minRunners: %d
  maxRunners: %d
%s
  githubConfigSecret: %s-secret
`, installation.Name, namespace, installation.Repository,
		installation.MinRunners, installation.MaxRunners, templateSpec, installation.Name)

	return manifest, nil
}

func generateStandardTemplate(installation *types.RunnerInstallation) string {
	volumeMounts := generateVolumeMounts(installation.CachePaths)
	volumes := generateVolumes(installation.CachePaths, installation.Name)

	template := `  template:
    spec:
      containers:
      - name: runner
        image: ghcr.io/actions/actions-runner:latest
        imagePullPolicy: Always`

	if volumeMounts != "" {
		template += "\n" + volumeMounts
	}

	if volumes != "" {
		template += "\n" + volumes
	}

	return template
}

func generatePrivilegedTemplate(installation *types.RunnerInstallation) string {
	volumeMounts := generateVolumeMounts(installation.CachePaths)
	volumes := generateVolumes(installation.CachePaths, installation.Name)

	template := `  template:
    spec:
      securityContext:
        runAsUser: 0
        runAsGroup: 0
        fsGroup: 0
      containers:
      - name: runner
        image: ghcr.io/actions/actions-runner:latest
        imagePullPolicy: Always
        securityContext:
          privileged: true
          runAsUser: 0
          runAsGroup: 0
          allowPrivilegeEscalation: true
          readOnlyRootFilesystem: false
          capabilities:
            add:
              - SYS_ADMIN
              - NET_ADMIN
              - SYS_PTRACE
              - SYS_CHROOT
              - SETFCAP
              - SETPCAP
              - NET_RAW
              - IPC_LOCK
              - SYS_RESOURCE
              - MKNOD
              - AUDIT_WRITE
              - AUDIT_CONTROL
        env:
        - name: SYSTEMD_IGNORE_CHROOT
          value: "1"`

	if volumeMounts != "" {
		template += "\n" + volumeMounts
	}

	if volumes != "" {
		template += "\n" + volumes
	}

	return template
}

func generateDinDTemplate(installation *types.RunnerInstallation) string {
	volumeMounts := generateVolumeMounts(installation.CachePaths)
	volumes := generateVolumes(installation.CachePaths, installation.Name)

	template := `  template:
    spec:
      containers:
      - name: runner
        image: ghcr.io/actions/actions-runner:latest
        imagePullPolicy: Always
        env:
        - name: DOCKER_HOST
          value: tcp://localhost:2376
        - name: DOCKER_TLS_VERIFY
          value: "1"
        - name: DOCKER_CERT_PATH
          value: /certs/client`

	if volumeMounts != "" {
		template += "\n" + volumeMounts
	}

	template += `
      - name: dind
        image: docker:dind
        securityContext:
          privileged: true
        env:
        - name: DOCKER_TLS_CERTDIR
          value: /certs
        volumeMounts:
        - name: docker-certs
          mountPath: /certs/client
        - name: dind-storage
          mountPath: /var/lib/docker`

	if volumes != "" {
		template += "\n" + volumes
	}

	template += `
      volumes:
      - name: docker-certs
        emptyDir: {}
      - name: dind-storage
        emptyDir: {}`

	return template
}

func generateVolumeMounts(cachePaths []types.CachePath) string {
	if len(cachePaths) == 0 {
		return ""
	}

	var mounts []string
	mounts = append(mounts, "        volumeMounts:")
	mounts = append(mounts, "        - name: work")
	mounts = append(mounts, "          mountPath: /home/runner/_work")

	for i, path := range cachePaths {
		mounts = append(mounts, fmt.Sprintf("        - name: cache-%d", i))
		mounts = append(mounts, fmt.Sprintf("          mountPath: %s", path.MountPath))
	}

	return strings.Join(mounts, "\n")
}

func generateVolumes(cachePaths []types.CachePath, installationName string) string {
	if len(cachePaths) == 0 {
		return ""
	}

	var volumes []string
	volumes = append(volumes, "      volumes:")
	volumes = append(volumes, "      - name: work")
	volumes = append(volumes, "        emptyDir: {}")

	for i, path := range cachePaths {
		hostPath := path.HostPath
		if hostPath == "" {
			// Generate default host path
			hostPath = fmt.Sprintf("/tmp/github-runner-cache/%s/cache-%d", installationName, i)
		}
		volumes = append(volumes, fmt.Sprintf("      - name: cache-%d", i))
		volumes = append(volumes, "        hostPath:")
		volumes = append(volumes, fmt.Sprintf("          path: %s", hostPath))
		volumes = append(volumes, "          type: DirectoryOrCreate")
	}

	return strings.Join(volumes, "\n")
}

// GenerateGitHubSecretManifest generates the secret manifest for GitHub authentication
func GenerateGitHubSecretManifest(installation *types.RunnerInstallation, namespace string) string {
	var secretData string

	switch installation.AuthType {
	case types.AuthTypeGitHubApp:
		secretData = fmt.Sprintf(`  github_app_id: %s
  github_app_installation_id: %s
  github_app_private_key: %s`,
			"", "", installation.AuthValue)
	case types.AuthTypePAT:
		secretData = fmt.Sprintf(`  github_token: %s`, installation.AuthValue)
	}

	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s-secret
  namespace: %s
type: Opaque
stringData:
%s
`, installation.Name, namespace, secretData)
}

// GenerateNamespaceManifest generates a namespace manifest
func GenerateNamespaceManifest(namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespace)
}
