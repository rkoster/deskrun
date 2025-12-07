package kapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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
	Silent bool   // Disable interactive prompts
	Color  bool   // Enable color output
	JSON   bool   // Output in JSON format
	Debug  bool   // Enable debug logging
}

// NewClient creates a new kapp client with default UI configuration
func NewClient(kubeconfig, namespace string) *Client {
	return NewClientWithUI(kubeconfig, namespace, UIConfig{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Silent: true, // Non-interactive by default
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
	// Build kapp command
	args := []string{
		"deploy",
		"-a", appName,
		"-f", manifestPath,
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"-y", // auto-confirm
		"--color=false",
		"--tty=false",
	}

	// Execute with UI output capture
	return c.execWithUI("kapp", args)
}

// Delete deletes an app using kapp
func (c *Client) Delete(appName string) error {
	// Build kapp command
	args := []string{
		"delete",
		"-a", appName,
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"-y", // auto-confirm
		"--color=false",
		"--tty=false",
	}

	// Execute with UI output capture
	return c.execWithUI("kapp", args)
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

// List lists all kapp apps using JSON output for reliable parsing
func (c *Client) List() ([]string, error) {
	// Use os/exec to run kapp directly
	cmd := exec.Command("kapp",
		"list",
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"--json",
		"--color=false",
		"--tty=false",
	)

	output, err := cmd.Output()
	if err != nil {
		// Get stderr for better error reporting
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			// If namespace doesn't exist, return empty list
			if strings.Contains(stderr, "not found") {
				return []string{}, nil
			}
			return nil, fmt.Errorf("kapp list failed: %w\nstderr: %s", err, stderr)
		}
		return nil, fmt.Errorf("kapp list failed: %w", err)
	}

	// Parse JSON output
	var listOutput KappListOutput
	if err := json.Unmarshal(output, &listOutput); err != nil {
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

// inspectWithFlags is a helper method that executes kapp inspect with custom flags
func (c *Client) inspectWithFlags(appName string, flags []string) (string, error) {
	// Build the full command args
	baseArgs := []string{
		"inspect",
		"-a", appName,
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"--color=false",
		"--tty=false",
	}
	args := append(baseArgs, flags...)

	// Use os/exec to run kapp directly
	cmd := exec.Command("kapp", args...)

	output, err := cmd.Output()
	if err != nil {
		// Get stderr for better error reporting
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("kapp inspect failed: %w\nstderr: %s", err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("kapp inspect failed: %w", err)
	}

	return string(output), nil
}

// InspectJSON gets the JSON output from kapp inspect with tree hierarchy and parses it
func (c *Client) InspectJSON(appName string) (*KappInspectOutput, error) {
	output, err := c.inspectWithFlags(appName, []string{"--json", "--tree"})
	if err != nil {
		return nil, err
	}

	// Parse JSON output
	var kappOutput KappInspectOutput
	if err := json.Unmarshal([]byte(output), &kappOutput); err != nil {
		return nil, fmt.Errorf("failed to parse kapp JSON output: %w", err)
	}

	return &kappOutput, nil
}

// execWithUI executes a command and captures output through the UI configuration
func (c *Client) execWithUI(command string, args []string) error {
	// Create command
	cmd := exec.Command(command, args...)

	// Create buffers for stdout and stderr
	var stdout, stderr bytes.Buffer

	// If custom writers are provided, use them; otherwise use buffers
	if c.uiConfig.Stdout != nil {
		cmd.Stdout = c.uiConfig.Stdout
	} else {
		cmd.Stdout = &stdout
	}

	if c.uiConfig.Stderr != nil {
		cmd.Stderr = c.uiConfig.Stderr
	} else {
		cmd.Stderr = &stderr
	}

	// Execute command
	err := cmd.Run()

	// Handle errors - if using buffers, format error with stderr content
	if err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%s failed: %w\nstderr: %s", command, err, stderr.String())
		}
		return fmt.Errorf("%s failed: %w", command, err)
	}

	return nil
}
