package templates

import (
	"embed"
	"io/fs"
	"path/filepath"
)

// Embedded ARC templates using ytt format
// These are vendored pre-rendered Helm charts converted to ytt templates
// with overlays for different container modes.

//go:embed all:templates
var embeddedFS embed.FS

// GetTemplateFS returns the embedded filesystem containing all templates
func GetTemplateFS() embed.FS {
	return embeddedFS
}

// GetTemplateFiles returns a map of filename -> content for all embedded templates
func GetTemplateFiles() (map[string]string, error) {
	files := map[string]string{}

	// Walk through all embedded files
	err := fs.WalkDir(embeddedFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and .gitkeep files
		if d.IsDir() || filepath.Base(path) == ".gitkeep" {
			return nil
		}

		// Read file content
		content, err := embeddedFS.ReadFile(path)
		if err != nil {
			return err
		}

		// Remove "templates/" prefix (10 chars)
		key := path
		if len(path) > 10 && path[:10] == "templates/" {
			key = path[10:]
		}

		files[key] = string(content)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// GetControllerChart returns the controller chart YAML
func GetControllerChart() (string, error) {
	content, err := embeddedFS.ReadFile("templates/controller/rendered.yaml")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// GetScaleSetChart returns the scale-set chart YAML
func GetScaleSetChart() (string, error) {
	content, err := embeddedFS.ReadFile("templates/scale-set/rendered.yaml")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// GetOverlay returns the specified overlay file content
func GetOverlay(filename string) (string, error) {
	content, err := embeddedFS.ReadFile("templates/overlays/" + filename)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// GetUniversalOverlay returns the universal overlay file that handles all container modes
func GetUniversalOverlay() (string, error) {
	content, err := embeddedFS.ReadFile("templates/overlay.yaml")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// GetSchema returns the data values schema
func GetSchema() (string, error) {
	content, err := embeddedFS.ReadFile("templates/values/schema.yaml")
	if err != nil {
		return "", err
	}
	return string(content), nil
}
