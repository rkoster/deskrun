package templates

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	cmdtpl "github.com/k14s/ytt/pkg/cmd/template"
	"github.com/k14s/ytt/pkg/cmd/ui"
	"github.com/k14s/ytt/pkg/files"
	"gopkg.in/yaml.v3"
)

// Processor handles template processing using the ytt Go library
type Processor struct{}

// NewProcessor creates a new template processor
func NewProcessor() *Processor {
	return &Processor{}
}

// ProcessTemplate processes templates based on the template type and configuration
// This is the main API for the unified template processing package
func (p *Processor) ProcessTemplate(templateType TemplateType, config Config) ([]byte, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, NewTemplateError(ErrorTypeValidation, err.Error(), err)
	}

	switch templateType {
	case TemplateTypeController:
		return p.processControllerTemplate(config)
	case TemplateTypeScaleSet:
		return p.processScaleSetTemplate(config)
	default:
		return nil, NewTemplateError(ErrorTypeValidation,
			fmt.Sprintf("unknown template type: %s", templateType), nil)
	}
}

// GetRawTemplate returns the raw template content without processing
// For scale-set templates, this returns the kubernetes base template as the default
func (p *Processor) GetRawTemplate(templateType TemplateType) ([]byte, error) {
	switch templateType {
	case TemplateTypeController:
		content, err := GetControllerChart()
		if err != nil {
			return nil, NewTemplateError(ErrorTypeIO, "failed to read controller template", err)
		}
		return []byte(content), nil
	case TemplateTypeScaleSet:
		// Return the kubernetes base template as the default raw template
		content, err := GetScaleSetBase("kubernetes")
		if err != nil {
			return nil, NewTemplateError(ErrorTypeIO, "failed to read scale-set template", err)
		}
		return []byte(content), nil
	default:
		return nil, NewTemplateError(ErrorTypeValidation,
			fmt.Sprintf("unknown template type: %s", templateType), nil)
	}
}

// processControllerTemplate processes the ARC controller template
func (p *Processor) processControllerTemplate(config Config) ([]byte, error) {
	content, err := GetControllerChart()
	if err != nil {
		return nil, NewTemplateError(ErrorTypeIO, "failed to read controller template", err).
			WithTemplate("controller/rendered.yaml")
	}
	// Controller template is static, no ytt processing needed
	return []byte(content), nil
}

// processScaleSetTemplate processes the scale-set template with ytt overlays
func (p *Processor) processScaleSetTemplate(config Config) ([]byte, error) {
	// Build input files for ytt
	inputFiles, err := p.buildInputFiles(config)
	if err != nil {
		return nil, err
	}

	// Process with ytt library
	return p.processWithYttLibrary(inputFiles, config)
}

// buildInputFiles creates the input files for ytt processing
func (p *Processor) buildInputFiles(config Config) ([]*files.File, error) {
	var inputFiles []*files.File

	// 1. Get the base scale-set template based on container mode (runtime selection)
	scaleSetContent, err := GetScaleSetBase(config.Installation.ContainerMode)
	if err != nil {
		return nil, NewTemplateError(ErrorTypeIO, "failed to read scale-set base template", err).
			WithTemplate(fmt.Sprintf("scale-set/bases/%s.yaml", config.Installation.ContainerMode))
	}

	// Transform static values to ytt data value expressions
	transformedTemplate := p.transformTemplateForYtt(scaleSetContent)

	templateFile := files.MustNewFileFromSource(
		files.NewBytesSource("scale-set.yaml", []byte(transformedTemplate)),
	)
	inputFiles = append(inputFiles, templateFile)

	// 2. Add the universal overlay (deskrun-specific customizations only)
	overlayContent, err := GetUniversalOverlay()
	if err != nil {
		return nil, NewTemplateError(ErrorTypeIO, "failed to read universal overlay", err).
			WithTemplate("overlay.yaml")
	}

	overlayFile := files.MustNewFileFromSource(
		files.NewBytesSource("overlay.yaml", []byte(overlayContent)),
	)
	inputFiles = append(inputFiles, overlayFile)

	// 3. Create data values file
	dataValuesYAML, err := p.buildDataValues(config)
	if err != nil {
		return nil, err
	}

	dataValuesFile := files.MustNewFileFromSource(
		files.NewBytesSource("data-values.yaml", dataValuesYAML),
	)
	// Mark as data values file (not a template)
	dataValuesFile.MarkType(files.TypeYAML)
	inputFiles = append(inputFiles, dataValuesFile)

	return inputFiles, nil
}

// transformTemplateForYtt transforms static template values to ytt data value expressions
func (p *Processor) transformTemplateForYtt(templateContent string) string {
	// Replace static values with ytt data value expressions - be specific to avoid partial matches
	result := strings.ReplaceAll(templateContent, "https://github.com/example/repo", "#@ data.values.installation.repository")
	result = strings.ReplaceAll(result, "arc-runner-gha-rs-github-secret", "#@ data.values.installation.name + \"-gha-rs-github-secret\"")
	result = strings.ReplaceAll(result, "arc-runner-gha-rs-no-permission", "#@ data.values.installation.name + \"-gha-rs-no-permission\"")
	result = strings.ReplaceAll(result, "arc-runner-gha-rs-manager", "#@ data.values.installation.name + \"-gha-rs-manager\"")
	result = strings.ReplaceAll(result, "arc-runner-gha-rs-kube-mode", "#@ data.values.installation.name + \"-gha-rs-kube-mode\"")

	// Replace remaining arc-runner references (labels, names, etc.)
	result = strings.ReplaceAll(result, "\"arc-runner\"", "#@ data.values.installation.name")
	result = strings.ReplaceAll(result, ": arc-runner", ": #@ data.values.installation.name")
	result = strings.ReplaceAll(result, "name: arc-runner", "name: #@ data.values.installation.name")
	result = strings.ReplaceAll(result, "cGxhY2Vob2xkZXI=", "#@ base64.encode(data.values.installation.authValue)")

	// Add ytt load directive at the beginning of the file
	result = "#@ load(\"@ytt:data\", \"data\")\n#@ load(\"@ytt:base64\", \"base64\")\n" + result

	return result
}

// buildDataValues creates the ytt data values YAML from the configuration
func (p *Processor) buildDataValues(config Config) ([]byte, error) {
	// Convert cache paths to simple map format for easier ytt access
	var cachePaths []map[string]string
	for _, cp := range config.Installation.CachePaths {
		cachePaths = append(cachePaths, map[string]string{
			"target": cp.Target,
			"source": cp.Source,
		})
	}

	// If no cache paths, use empty array (not nil) for ytt
	if cachePaths == nil {
		cachePaths = []map[string]string{}
	}

	dataValues := map[string]any{
		"installation": map[string]any{
			"name":          config.InstanceName,
			"repository":    config.Installation.Repository,
			"authValue":     config.Installation.AuthValue,
			"containerMode": string(config.Installation.ContainerMode),
			"minRunners":    config.Installation.MinRunners,
			"maxRunners":    config.Installation.MaxRunners,
			"cachePaths":    cachePaths,
			"instanceNum":   config.InstanceNum,
		},
	}

	yamlBytes, err := yaml.Marshal(dataValues)
	if err != nil {
		return nil, NewTemplateError(ErrorTypeData, "failed to marshal data values", err).
			WithContext(dataValues)
	}

	// Add ytt data values header
	header := "#@data/values\n---\n"
	return append([]byte(header), yamlBytes...), nil
}

// processWithYttLibrary uses the ytt Go library to process templates
// This is the key function that AVOIDS shell execution
func (p *Processor) processWithYttLibrary(inputFiles []*files.File, config Config) ([]byte, error) {
	// Create ytt options
	opts := cmdtpl.NewOptions()
	opts.IgnoreUnknownComments = true

	// Sort files to ensure consistent ordering
	sortedFiles := files.NewSortedFiles(inputFiles)

	// Create input
	input := cmdtpl.Input{
		Files: sortedFiles,
	}

	// Create a custom UI that captures output (no TTY needed)
	customUI := ui.NewCustomWriterTTY(false, &bytes.Buffer{}, &bytes.Buffer{})

	// Run ytt
	output := opts.RunWithFiles(input, customUI)

	// Check for errors
	if output.Err != nil {
		return nil, p.parseYttError(output.Err, config)
	}

	// Convert output to YAML bytes
	if output.DocSet == nil {
		return nil, NewTemplateError(ErrorTypeUnknown, "ytt returned no output", nil)
	}

	// Render the output to YAML
	var result bytes.Buffer
	for i, doc := range output.DocSet.Items {
		if i > 0 {
			result.WriteString("---\n")
		}
		docBytes, err := doc.AsYAMLBytes()
		if err != nil {
			return nil, NewTemplateError(ErrorTypeUnknown,
				fmt.Sprintf("failed to render document %d", i), err)
		}
		result.Write(docBytes)
	}

	return result.Bytes(), nil
}

// parseYttError parses ytt error output and creates a detailed TemplateError
func (p *Processor) parseYttError(err error, config Config) *TemplateError {
	errStr := err.Error()

	// Try to parse line/column information from error
	// ytt errors typically look like: "file.yaml:10:5: error message"
	re := regexp.MustCompile(`([^:]+):(\d+):(\d+):\s*(.+)`)
	matches := re.FindStringSubmatch(errStr)

	templateErr := NewTemplateError(ErrorTypeUnknown, errStr, err)

	if len(matches) >= 5 {
		templateErr.Template = matches[1]
		if line, parseErr := strconv.Atoi(matches[2]); parseErr == nil {
			templateErr.Line = line
		}
		if col, parseErr := strconv.Atoi(matches[3]); parseErr == nil {
			templateErr.Column = col
		}
		templateErr.Message = matches[4]

		// Classify error type
		if strings.Contains(errStr, "overlay") {
			templateErr.Type = ErrorTypeOverlay
		} else if strings.Contains(errStr, "syntax") || strings.Contains(errStr, "parse") {
			templateErr.Type = ErrorTypeSyntax
		} else if strings.Contains(errStr, "data") {
			templateErr.Type = ErrorTypeData
		}
	}

	// Add context information
	context := map[string]any{
		"instanceName":  config.InstanceName,
		"instanceNum":   config.InstanceNum,
		"containerMode": string(config.Installation.ContainerMode),
		"repository":    config.Installation.Repository,
	}
	templateErr.WithContext(context)

	return templateErr
}
