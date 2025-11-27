package templates

import (
	"embed"
)

// Embedded ARC templates using ytt format
// These are vendored pre-rendered Helm charts converted to ytt templates
// with overlays for different container modes.

//go:embed all:config/arc
var embeddedFS embed.FS

// GetTemplateFiles returns a map of filename -> content for all embedded templates
func GetTemplateFiles() (map[string]string, error) {
	files := map[string]string{}
	
	templatePaths := []string{
		"config/arc/config.yaml",
		"config/arc/values/schema.yaml",
		"config/arc/_ytt_lib/controller/rendered.yaml",
		"config/arc/_ytt_lib/scale-set/rendered.yaml",
		"config/arc/overlays/container-mode-kubernetes.yaml",
		"config/arc/overlays/container-mode-privileged.yaml",
		"config/arc/overlays/container-mode-dind.yaml",
	}
	
	for _, path := range templatePaths {
		content, err := embeddedFS.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// Remove the "config/arc/" prefix for the key
		key := path[11:] // len("config/arc/") = 11
		files[key] = string(content)
	}
	
	return files, nil
}

// GetControllerChart returns the controller chart YAML
func GetControllerChart() (string, error) {
	content, err := embeddedFS.ReadFile("config/arc/_ytt_lib/controller/rendered.yaml")
	if err != nil {
		return "", err
	}
	return string(content), nil
}
