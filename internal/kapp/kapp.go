package kapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	cmdapp "carvel.dev/kapp/pkg/kapp/cmd/app"
	cmdcore "carvel.dev/kapp/pkg/kapp/cmd/core"
	"carvel.dev/kapp/pkg/kapp/logger"
	"carvel.dev/kapp/pkg/kapp/preflight"
	"github.com/cppforlife/go-cli-ui/ui"
)

// Client provides an interface for kapp operations
type Client struct {
	kubeconfig string
	namespace  string
	uiConfig   UIConfig
}

// KappResource represents a single resource from kapp JSON output
type KappResource struct {
	Age            string `json:"age"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Owner          string `json:"owner"`
	ReconcileInfo  string `json:"reconcile_info"`
	ReconcileState string `json:"reconcile_state"`
}

// KappTable represents the table structure in kapp JSON output
type KappTable struct {
	Content string            `json:"Content"`
	Header  map[string]string `json:"Header"`
	Rows    []KappResource    `json:"Rows"`
}

// KappInspectOutput represents the full kapp JSON output
type KappInspectOutput struct {
	Tables []KappTable `json:"Tables"`
}

// UIConfig holds configuration for UI behavior
type UIConfig struct {
	Stdout io.Writer
	Stderr io.Writer
	Silent bool // Disable interactive prompts
	Color  bool // Enable color output
	JSON   bool // Output in JSON format
	Debug  bool // Enable debug logging
}

// NewClient creates a new kapp client with default UI configuration
func NewClient(kubeconfig, namespace string) *Client {
	return NewClientWithUI(kubeconfig, namespace, UIConfig{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Silent: true,  // Non-interactive by default
		Color:  false, // No color by default
		JSON:   false,
		Debug:  false,
	})
}

// NewClientWithUI creates a new kapp client with custom UI configuration
func NewClientWithUI(kubeconfig, namespace string, uiConfig UIConfig) *Client {
	return &Client{
		kubeconfig: kubeconfig,
		namespace:  namespace,
		uiConfig:   uiConfig,
	}
}

// Deploy deploys resources using kapp
func (c *Client) Deploy(appName string, manifestPath string) error {
	// Create a custom UI with the configured writers
	confUI := c.createConfUI()

	// Create kapp dependencies
	configFactory := cmdcore.NewConfigFactoryImpl()
	depsFactory := cmdcore.NewDepsFactoryImpl(configFactory, confUI)
	preflights := preflight.NewRegistry(map[string]preflight.Check{})

	// Configure kubeconfig context
	configFactory.ConfigureContextResolver(func() (string, error) {
		return c.kubeconfig, nil
	})

	// Create deploy options
	deployOpts := cmdapp.NewDeployOptions(confUI, depsFactory, logger.NewUILogger(confUI), preflights)

	// Set the required flags programmatically
	deployOpts.AppFlags.Name = appName
	deployOpts.AppFlags.NamespaceFlags.Name = c.namespace
	deployOpts.FileFlags.Files = []string{manifestPath}

	// Enable non-interactive mode
	confUI.EnableNonInteractive()

	// Execute deploy
	return deployOpts.Run()
}

// Delete deletes an app using kapp
func (c *Client) Delete(appName string) error {
	// Create a custom UI with the configured writers
	confUI := c.createConfUI()

	// Create kapp dependencies
	configFactory := cmdcore.NewConfigFactoryImpl()
	depsFactory := cmdcore.NewDepsFactoryImpl(configFactory, confUI)

	// Configure kubeconfig context
	configFactory.ConfigureContextResolver(func() (string, error) {
		return c.kubeconfig, nil
	})

	// Create delete options
	deleteOpts := cmdapp.NewDeleteOptions(confUI, depsFactory, logger.NewUILogger(confUI))

	// Set the required flags programmatically
	deleteOpts.AppFlags.Name = appName
	deleteOpts.AppFlags.NamespaceFlags.Name = c.namespace

	// Enable non-interactive mode
	confUI.EnableNonInteractive()

	// Execute delete
	return deleteOpts.Run()
}

// KappListApp represents a single app from kapp list JSON output
type KappListApp struct {
	Name string `json:"name"`
}

// KappListTable represents the table structure in kapp list JSON output
type KappListTable struct {
	Rows []KappListApp `json:"Rows"`
}

// KappListOutput represents the full kapp list JSON output
type KappListOutput struct {
	Tables []KappListTable `json:"Tables"`
}

// List lists all kapp apps using the native kapp Go API
func (c *Client) List() ([]string, error) {
	// Create a buffer to capture JSON output
	var outputBuf bytes.Buffer

	// Create a temporary UI configuration with the buffer
	writerUI := ui.NewWriterUI(&outputBuf, io.Discard, ui.NewNoopLogger())
	confUI := ui.NewWrappingConfUI(writerUI, ui.NewNoopLogger())
	confUI.EnableNonInteractive()
	confUI.EnableJSON()

	// Create kapp dependencies
	configFactory := cmdcore.NewConfigFactoryImpl()
	depsFactory := cmdcore.NewDepsFactoryImpl(configFactory, confUI)

	// Configure kubeconfig context
	configFactory.ConfigureContextResolver(func() (string, error) {
		return c.kubeconfig, nil
	})

	// Create list options
	listOpts := cmdapp.NewListOptions(confUI, depsFactory, logger.NewUILogger(confUI))

	// Set the required flags programmatically
	listOpts.NamespaceFlags.Name = c.namespace

	// Execute list
	err := listOpts.Run()
	if err != nil {
		// Check if namespace doesn't exist
		if strings.Contains(err.Error(), "not found") {
			return []string{}, nil
		}
		return nil, fmt.Errorf("kapp list failed: %w", err)
	}

	// Parse JSON output
	var listOutput KappListOutput
	if err := json.Unmarshal(outputBuf.Bytes(), &listOutput); err != nil {
		return nil, fmt.Errorf("failed to parse kapp list JSON output: %w", err)
	}

	// Extract app names from JSON
	var names []string
	if len(listOutput.Tables) > 0 {
		for _, app := range listOutput.Tables[0].Rows {
			if app.Name != "" {
				names = append(names, app.Name)
			}
		}
	}

	return names, nil
}

// InspectJSON gets the JSON output from kapp inspect with tree hierarchy using native kapp Go API
func (c *Client) InspectJSON(appName string) (*KappInspectOutput, error) {
	// Create a buffer to capture JSON output
	var outputBuf bytes.Buffer

	// Create a temporary UI configuration with the buffer
	writerUI := ui.NewWriterUI(&outputBuf, io.Discard, ui.NewNoopLogger())
	confUI := ui.NewWrappingConfUI(writerUI, ui.NewNoopLogger())
	confUI.EnableNonInteractive()
	confUI.EnableJSON()

	// Create kapp dependencies
	configFactory := cmdcore.NewConfigFactoryImpl()
	depsFactory := cmdcore.NewDepsFactoryImpl(configFactory, confUI)

	// Configure kubeconfig context
	configFactory.ConfigureContextResolver(func() (string, error) {
		return c.kubeconfig, nil
	})

	// Create inspect options
	inspectOpts := cmdapp.NewInspectOptions(confUI, depsFactory, logger.NewUILogger(confUI))

	// Set the required flags programmatically
	inspectOpts.AppFlags.Name = appName
	inspectOpts.AppFlags.NamespaceFlags.Name = c.namespace
	inspectOpts.Tree = true

	// Execute inspect
	err := inspectOpts.Run()
	if err != nil {
		return nil, fmt.Errorf("kapp inspect failed: %w", err)
	}

	// Parse JSON output
	var kappOutput KappInspectOutput
	if err := json.Unmarshal(outputBuf.Bytes(), &kappOutput); err != nil {
		return nil, fmt.Errorf("failed to parse kapp JSON output: %w", err)
	}

	return &kappOutput, nil
}

// createConfUI creates a go-cli-ui ConfUI based on the client's UI configuration
func (c *Client) createConfUI() *ui.ConfUI {
	// Determine output and error writers
	outWriter := c.uiConfig.Stdout
	if outWriter == nil {
		outWriter = os.Stdout
	}

	errWriter := c.uiConfig.Stderr
	if errWriter == nil {
		errWriter = os.Stderr
	}

	// Create a writer UI with custom writers
	writerUI := ui.NewWriterUI(outWriter, errWriter, ui.NewNoopLogger())

	// Wrap in ConfUI for configuration
	confUI := ui.NewWrappingConfUI(writerUI, ui.NewNoopLogger())

	// Apply UI configuration
	if c.uiConfig.Color {
		confUI.EnableColor()
	}

	if c.uiConfig.JSON {
		confUI.EnableJSON()
	}

	if c.uiConfig.Silent {
		confUI.EnableNonInteractive()
	}

	return confUI
}
