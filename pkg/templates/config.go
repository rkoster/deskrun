package templates

import (
	"fmt"

	"github.com/rkoster/deskrun/pkg/types"
)

// TemplateType represents the type of template to process
type TemplateType string

const (
	// TemplateTypeController is the ARC controller template
	TemplateTypeController TemplateType = "controller"
	// TemplateTypeScaleSet is the runner scale-set template
	TemplateTypeScaleSet TemplateType = "scale-set"
)

// Config contains configuration for template processing
type Config struct {
	// Runner installation configuration
	Installation *types.RunnerInstallation

	// Instance details
	InstanceName string
	InstanceNum  int

	// Optional: Override defaults
	Namespace string // default: "arc-systems"
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Installation == nil {
		return fmt.Errorf("installation configuration is required")
	}
	if c.InstanceName == "" {
		return fmt.Errorf("instance name is required")
	}
	if c.Installation.Repository == "" {
		return fmt.Errorf("repository URL is required")
	}
	if c.Installation.ContainerMode == "" {
		return fmt.Errorf("container mode is required")
	}

	// Validate container mode is one of the known values
	switch c.Installation.ContainerMode {
	case types.ContainerModeKubernetes, types.ContainerModeDinD, types.ContainerModePrivileged:
		// Valid
	default:
		return fmt.Errorf("invalid container mode: %s (must be one of: kubernetes, dind, cached-privileged-kubernetes)", c.Installation.ContainerMode)
	}

	return nil
}

// GetNamespace returns the namespace, using "arc-systems" as default
func (c *Config) GetNamespace() string {
	if c.Namespace == "" {
		return "arc-systems"
	}
	return c.Namespace
}

// ErrorType represents the type of template processing error
type ErrorType string

const (
	// ErrorTypeSyntax indicates a ytt syntax error
	ErrorTypeSyntax ErrorType = "syntax"
	// ErrorTypeOverlay indicates an overlay application error
	ErrorTypeOverlay ErrorType = "overlay"
	// ErrorTypeData indicates a data values error
	ErrorTypeData ErrorType = "data"
	// ErrorTypeValidation indicates a validation error
	ErrorTypeValidation ErrorType = "validation"
	// ErrorTypeIO indicates an I/O error (file read/write)
	ErrorTypeIO ErrorType = "io"
	// ErrorTypeUnknown indicates an unknown error
	ErrorTypeUnknown ErrorType = "unknown"
)

// TemplateError provides verbose error information for template processing failures
type TemplateError struct {
	// Type classifies the error
	Type ErrorType

	// Template is the name of the template that caused the error
	Template string

	// Line is the line number where the error occurred (if available)
	Line int

	// Column is the column number where the error occurred (if available)
	Column int

	// Message is the human-readable error message
	Message string

	// YttOutput contains the full ytt output for debugging
	YttOutput string

	// Context contains the configuration that caused the error
	Context map[string]interface{}

	// Cause is the underlying error
	Cause error
}

// Error implements the error interface
func (e *TemplateError) Error() string {
	if e.Line > 0 && e.Column > 0 {
		return fmt.Sprintf("%s error in %s at line %d, column %d: %s", e.Type, e.Template, e.Line, e.Column, e.Message)
	}
	if e.Line > 0 {
		return fmt.Sprintf("%s error in %s at line %d: %s", e.Type, e.Template, e.Line, e.Message)
	}
	if e.Template != "" {
		return fmt.Sprintf("%s error in %s: %s", e.Type, e.Template, e.Message)
	}
	return fmt.Sprintf("%s error: %s", e.Type, e.Message)
}

// Unwrap returns the underlying error
func (e *TemplateError) Unwrap() error {
	return e.Cause
}

// VerboseError returns a detailed error message for debugging
func (e *TemplateError) VerboseError() string {
	result := "Template Processing Error\n"
	result += "========================\n"
	result += fmt.Sprintf("Type: %s\n", e.Type)
	if e.Template != "" {
		result += fmt.Sprintf("Template: %s\n", e.Template)
	}
	if e.Line > 0 {
		result += fmt.Sprintf("Line: %d\n", e.Line)
	}
	if e.Column > 0 {
		result += fmt.Sprintf("Column: %d\n", e.Column)
	}
	result += fmt.Sprintf("Message: %s\n", e.Message)
	if e.YttOutput != "" {
		result += fmt.Sprintf("\nYTT Output:\n%s\n", e.YttOutput)
	}
	if len(e.Context) > 0 {
		result += "\nConfiguration Context:\n"
		for k, v := range e.Context {
			result += fmt.Sprintf("  %s: %v\n", k, v)
		}
	}
	if e.Cause != nil {
		result += fmt.Sprintf("\nUnderlying Error: %v\n", e.Cause)
	}
	return result
}

// NewTemplateError creates a new TemplateError
func NewTemplateError(errType ErrorType, message string, cause error) *TemplateError {
	return &TemplateError{
		Type:    errType,
		Message: message,
		Cause:   cause,
	}
}

// WithTemplate adds template information to the error
func (e *TemplateError) WithTemplate(template string) *TemplateError {
	e.Template = template
	return e
}

// WithLocation adds line and column information to the error
func (e *TemplateError) WithLocation(line, column int) *TemplateError {
	e.Line = line
	e.Column = column
	return e
}

// WithYttOutput adds ytt output to the error
func (e *TemplateError) WithYttOutput(output string) *TemplateError {
	e.YttOutput = output
	return e
}

// WithContext adds configuration context to the error
func (e *TemplateError) WithContext(context map[string]interface{}) *TemplateError {
	e.Context = context
	return e
}
