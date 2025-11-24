package types

// ContainerMode represents the different container modes for runners
type ContainerMode string

const (
	// ContainerModeKubernetes is the standard kubernetes mode
	ContainerModeKubernetes ContainerMode = "kubernetes"
	// ContainerModeDinD is Docker-in-Docker mode
	ContainerModeDinD ContainerMode = "dind"
	// ContainerModePrivileged is privileged kubernetes mode with special capabilities
	ContainerModePrivileged ContainerMode = "cached-privileged-kubernetes"
)

// RunnerInstallation represents a runner installation configuration
type RunnerInstallation struct {
	Name          string
	Repository    string
	ContainerMode ContainerMode
	MinRunners    int
	MaxRunners    int
	Instances     int // Number of separate runner scale set instances to create
	CachePaths    []CachePath
	AuthType      AuthType
	AuthValue     string
}

// CachePath represents a path to be cached using hostPath volumes
type CachePath struct {
	MountPath string
	HostPath  string
}

// AuthType represents the authentication type
type AuthType string

const (
	AuthTypeGitHubApp AuthType = "github-app"
	AuthTypePAT       AuthType = "pat"
)

// ClusterConfig represents the kind cluster configuration
type ClusterConfig struct {
	Name      string
	Network   string
	NixStore  *NixMount // Optional nix store mount
	NixSocket *NixMount // Optional nix socket mount
}

// NixMount represents a nix-related mount configuration
type NixMount struct {
	HostPath      string // Host path to mount from
	ContainerPath string // Container path to mount to
}
