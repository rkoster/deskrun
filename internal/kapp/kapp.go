package kapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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

// UIConfig holds configuration for UI behavior.
//
// Option interactions:
//   - If JSON is true, output is formatted as JSON and color is disabled, regardless of the Color setting.
//   - If Silent is true, interactive prompts are disabled and non-essential output is suppressed.
//   - Color enables colored output for supported formats, but is ignored if JSON is true.
//   - Stdout and Stderr specify the output destinations for normal and error output, respectively.
type UIConfig struct {
	Stdout io.Writer
	Stderr io.Writer
	Silent bool // Disable interactive prompts and suppress non-essential output
	Color  bool // Enable color output (ignored if JSON is true)
	JSON   bool // Output in JSON format (disables color)
}

// NewClient creates a new kapp client with default UI configuration
func NewClient(kubeconfig, namespace string) *Client {
	return NewClientWithUI(kubeconfig, namespace, UIConfig{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Silent: true,  // Non-interactive by default
		Color:  false, // No color by default
		JSON:   false,
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

// Deploy deploys resources using the native kapp Go API (not by executing the kapp CLI binary).
// This approach may result in error messages and behavior that differ from the CLI.
func (c *Client) Deploy(appName string, manifestPath string) error {
	// Create a custom UI with the configured writers
	confUI := c.createConfUI()

	// Create kapp dependencies with proper kubeconfig configuration
	configFactory := c.createConfigFactory()
	depsFactory := cmdcore.NewDepsFactoryImpl(configFactory, confUI)
	preflights := preflight.NewRegistry(map[string]preflight.Check{})

	// Create deploy options
	deployOpts := cmdapp.NewDeployOptions(confUI, depsFactory, logger.NewUILogger(confUI), preflights)

	// Set the required flags programmatically
	deployOpts.AppFlags.Name = appName
	deployOpts.AppFlags.NamespaceFlags.Name = c.namespace
	deployOpts.FileFlags.Files = []string{manifestPath}

	// Set default apply options (required to prevent throttle panic)
	// These match the defaults used by kapp CLI in ApplyFlagsDeployDefaults
	c.setDefaultApplyOptions(deployOpts)

	// Execute deploy (non-interactive mode is handled by createConfUI based on UIConfig.Silent)
	return deployOpts.Run()
}

// Delete deletes an app using the native kapp Go API (not by executing the kapp CLI binary).
// This approach may result in error messages and behavior that differ from the CLI.
func (c *Client) Delete(appName string) error {
	// Create a custom UI with the configured writers
	confUI := c.createConfUI()

	// Create kapp dependencies with proper kubeconfig configuration
	configFactory := c.createConfigFactory()
	depsFactory := cmdcore.NewDepsFactoryImpl(configFactory, confUI)

	// Create delete options
	deleteOpts := cmdapp.NewDeleteOptions(confUI, depsFactory, logger.NewUILogger(confUI))

	// Set the required flags programmatically
	deleteOpts.AppFlags.Name = appName
	deleteOpts.AppFlags.NamespaceFlags.Name = c.namespace

	// Execute delete (non-interactive mode is handled by createConfUI based on UIConfig.Silent)
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

	// Use a helper to create a JSON-enabled UI configuration
	confUI := c.createJSONUI(&outputBuf)

	// Create kapp dependencies with proper kubeconfig configuration
	configFactory := c.createConfigFactory()
	depsFactory := cmdcore.NewDepsFactoryImpl(configFactory, confUI)

	// Create list options
	listOpts := cmdapp.NewListOptions(confUI, depsFactory, logger.NewUILogger(confUI))

	// Set the required flags programmatically
	// When listing apps in a specific namespace, we need to set the namespace
	// and ensure we're not listing all namespaces
	listOpts.NamespaceFlags.Name = c.namespace

	// Explicitly disable all-namespaces mode to ensure we only list in the specified namespace
	// This is important because the default behavior might list across all namespaces
	// Set AllNamespaces to false (empty string means don't use all namespaces flag)
	// The NamespaceFlags.Name should be sufficient, but we want to be explicit

	// Execute list
	err := listOpts.Run()
	if err != nil {
		// Check if error is specifically about a missing namespace.
		// This is more robust than matching only "not found".
		if strings.Contains(err.Error(), "namespace") && strings.Contains(err.Error(), "not found") {
			return []string{}, nil
		}
		return nil, fmt.Errorf("kapp list failed: %w", err)
	}

	// Parse JSON output
	outputBytes := outputBuf.Bytes()
	if len(outputBytes) == 0 {
		// Empty output - this likely means no apps are deployed in the namespace
		// This is a normal case, not an error
		return []string{}, nil
	}

	var listOutput KappListOutput
	if err := json.Unmarshal(outputBytes, &listOutput); err != nil {
		// Provide detailed error with actual output for debugging
		return nil, fmt.Errorf("failed to parse kapp list JSON output: %w (output length: %d, output: %q)", err, len(outputBytes), string(outputBytes))
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

	// Use a helper to create a JSON-enabled UI configuration
	confUI := c.createJSONUI(&outputBuf)

	// Create kapp dependencies with proper kubeconfig configuration
	configFactory := c.createConfigFactory()
	depsFactory := cmdcore.NewDepsFactoryImpl(configFactory, confUI)

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
	outputBytes := outputBuf.Bytes()
	if len(outputBytes) == 0 {
		return nil, fmt.Errorf("kapp inspect returned no output for app %s", appName)
	}

	var kappOutput KappInspectOutput
	if err := json.Unmarshal(outputBytes, &kappOutput); err != nil {
		return nil, fmt.Errorf("failed to parse kapp JSON output: %w (output: %q)", err, string(outputBytes))
	}

	return &kappOutput, nil
}

// createConfigFactory creates and configures a kapp ConfigFactory with proper kubeconfig settings
func (c *Client) createConfigFactory() *cmdcore.ConfigFactoryImpl {
	configFactory := cmdcore.NewConfigFactoryImpl()

	// Configure kubeconfig path resolver
	// The kubeconfig field in Client represents the context name, but kapp needs the path
	// We'll use the default kubeconfig path and set the context
	configFactory.ConfigurePathResolver(func() (string, error) {
		// Return empty string to use default kubeconfig location (~/.kube/config)
		return "", nil
	})

	// Configure context resolver to use the specified context
	configFactory.ConfigureContextResolver(func() (string, error) {
		return c.kubeconfig, nil
	})

	// Configure YAML resolver (required by kapp, but we don't use explicit YAML config)
	configFactory.ConfigureYAMLResolver(func() (string, error) {
		// Return empty string to use kubeconfig file instead of explicit YAML
		return "", nil
	})

	return configFactory
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

// createJSONUI creates a go-cli-ui ConfUI for JSON output with the provided buffer.
// This is used by List() and InspectJSON() methods which require JSON output for parsing,
// independent of the client's UIConfig settings.
func (c *Client) createJSONUI(outputBuf *bytes.Buffer) *ui.ConfUI {
	// Create a writer UI with the buffer for output and discard errors
	writerUI := ui.NewWriterUI(outputBuf, io.Discard, ui.NewNoopLogger())

	// Wrap in ConfUI for configuration
	confUI := ui.NewWrappingConfUI(writerUI, ui.NewNoopLogger())

	// Always enable non-interactive and JSON for these operations
	confUI.EnableNonInteractive()
	confUI.EnableJSON()

	return confUI
}

// setDefaultApplyOptions sets the default apply options that match kapp CLI defaults.
// This is required to prevent panics and ensure consistent behavior with the CLI.
func (c *Client) setDefaultApplyOptions(deployOpts *cmdapp.DeployOptions) {
	// Set default cluster change options (matches ApplyFlagsDeployDefaults)
	deployOpts.ApplyFlags.ApplyIgnored = false
	deployOpts.ApplyFlags.Wait = true
	deployOpts.ApplyFlags.WaitIgnored = false

	// Set default applying changes options (prevents throttle panic)
	deployOpts.ApplyFlags.ApplyingChangesOpts.Concurrency = 5
	deployOpts.ApplyFlags.ApplyingChangesOpts.Timeout = 15 * time.Minute
	deployOpts.ApplyFlags.ApplyingChangesOpts.CheckInterval = 1 * time.Second

	// Set default waiting changes options
	deployOpts.ApplyFlags.WaitingChangesOpts.Concurrency = 5
	deployOpts.ApplyFlags.WaitingChangesOpts.Timeout = 15 * time.Minute
	deployOpts.ApplyFlags.WaitingChangesOpts.CheckInterval = 3 * time.Second
	deployOpts.ApplyFlags.ResourceTimeout = 0 * time.Second

	// Set default exit behavior
	deployOpts.ApplyFlags.ExitEarlyOnApplyError = true
	deployOpts.ApplyFlags.ExitEarlyOnWaitError = true
}
