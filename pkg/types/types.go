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
	Mounts        []Mount
	CachePaths    []CachePath // Deprecated: Use Mounts instead. Kept for backward compatibility.
	AuthType      AuthType
	AuthValue     string
}

// MountType represents the type of host mount
type MountType string

const (
	// MountTypeDirectoryOrCreate creates a directory if it doesn't exist
	MountTypeDirectoryOrCreate MountType = "DirectoryOrCreate"
	// MountTypeDirectory mounts an existing directory
	MountTypeDirectory MountType = "Directory"
	// MountTypeSocket mounts a Unix socket
	MountTypeSocket MountType = "Socket"
)

// Mount represents a host path to be mounted into pods
type Mount struct {
	// Source path on the host machine (empty means auto-generated for DirectoryOrCreate)
	Source string
	// Target path inside the container where the mount will be mounted
	Target string
	// Type specifies the hostPath volume type (defaults to DirectoryOrCreate)
	Type MountType
}

// CachePath represents a path to be cached using hostPath volumes
// Deprecated: Use Mount instead. This type is kept for backward compatibility.
type CachePath struct {
	// Target path inside the container where the cache will be mounted
	Target string
	// Source path on the host machine (empty means auto-generated)
	Source string
}

// AuthType represents the authentication type
type AuthType string

const (
	AuthTypeGitHubApp AuthType = "github-app"
	AuthTypePAT       AuthType = "pat"
)

// ClusterConfig represents the kind cluster configuration
type ClusterConfig struct {
	Name         string
	Network      string
	NixStore     *Mount // Optional nix store mount
	NixSocket    *Mount // Optional nix socket mount
	DeskrunCache *Mount // Optional deskrun cache mount
}

// Mount represents a host-to-container mount configuration
type Mount struct {
	HostPath      string // Host path to mount from
	ContainerPath string // Container path to mount to
}

// NixMount is deprecated: use Mount instead
// Kept for backward compatibility
type NixMount = Mount
