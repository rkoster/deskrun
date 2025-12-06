package kapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	kappcmd "carvel.dev/kapp/pkg/kapp/cmd"
	"github.com/cppforlife/go-cli-ui/ui"
)

// Client provides an interface for kapp and ytt operations
type Client struct {
	kubeconfig string
	namespace  string
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

// NewClient creates a new kapp client
func NewClient(kubeconfig, namespace string) *Client {
	return &Client{
		kubeconfig: kubeconfig,
		namespace:  namespace,
	}
}

// ProcessTemplate executes ytt to process templates with data values
func (c *Client) ProcessTemplate(templateDir string, dataValuesPath string) (string, error) {
	// Use os/exec to run ytt directly, as the ytt library seems to have output redirection issues
	cmd := exec.Command("ytt",
		"-f", templateDir,
		"--data-values-file", dataValuesPath,
	)

	output, err := cmd.Output()
	if err != nil {
		// Get stderr for better error reporting
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("ytt failed: %w\nstderr: %s", err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("ytt failed: %w", err)
	}

	result := string(output)

	return result, nil
}

// Deploy deploys resources using kapp
func (c *Client) Deploy(appName string, manifestPath string) error {
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()

	// Create the kapp command
	kappCommand := kappcmd.NewDefaultKappCmd(confUI)

	// Set the command args
	kappCommand.SetArgs([]string{
		"deploy",
		"-a", appName,
		"-f", manifestPath,
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"-y", // auto-confirm
		"--color=false",
		"--tty=false",
	})

	// Capture output
	kappCommand.SetOut(&outBuf)
	kappCommand.SetErr(&errBuf)

	// Execute the command
	if err := kappCommand.Execute(); err != nil {
		return fmt.Errorf("kapp deploy failed: %w\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}

	return nil
}

// Delete deletes an app using kapp
func (c *Client) Delete(appName string) error {
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()

	// Create the kapp command
	kappCommand := kappcmd.NewDefaultKappCmd(confUI)

	// Set the command args
	kappCommand.SetArgs([]string{
		"delete",
		"-a", appName,
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"-y", // auto-confirm
		"--color=false",
		"--tty=false",
	})

	// Capture output
	kappCommand.SetOut(&outBuf)
	kappCommand.SetErr(&errBuf)

	// Execute the command
	if err := kappCommand.Execute(); err != nil {
		return fmt.Errorf("kapp delete failed: %w\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}

	return nil
}

// List lists all kapp apps
func (c *Client) List() ([]string, error) {
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()

	// Create the kapp command
	kappCommand := kappcmd.NewDefaultKappCmd(confUI)

	// Set the command args
	kappCommand.SetArgs([]string{
		"list",
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"--color=false",
		"--tty=false",
	})

	// Capture output
	kappCommand.SetOut(&outBuf)
	kappCommand.SetErr(&errBuf)

	// Execute the command
	if err := kappCommand.Execute(); err != nil {
		// If namespace doesn't exist, return empty list
		if strings.Contains(errBuf.String(), "not found") {
			return []string{}, nil
		}
		return nil, fmt.Errorf("kapp list failed: %w\nstderr: %s", err, errBuf.String())
	}

	// Parse the plain text output manually
	lines := strings.Split(strings.TrimSpace(outBuf.String()), "\n")
	var names []string

	// Skip header line and parse app names
	for i, line := range lines {
		if i == 0 && strings.Contains(line, "Name") {
			continue // Skip header
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] != "" {
			names = append(names, fields[0])
		}
	}

	return names, nil
}

// inspectWithFlags is a helper method that executes kapp inspect with custom flags
//
// Note: This uses os/exec to run the kapp CLI directly instead of the kapp Go library
// because the library's inspect command with --json doesn't properly write to the output
// buffers set via SetOut(). The library appears to write JSON output through a different
// mechanism that we cannot easily capture. This is similar to how ProcessTemplate() uses
// os/exec for ytt. Both kapp and ytt are required to be in PATH for deskrun to work.
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

	// Use os/exec to run kapp CLI directly
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

// InspectJSON gets the JSON output from kapp inspect and parses it
func (c *Client) InspectJSON(appName string) (*KappInspectOutput, error) {
	output, err := c.inspectWithFlags(appName, []string{"--json"})
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
