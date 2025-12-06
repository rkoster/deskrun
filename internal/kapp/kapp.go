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
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()

	// Create the kapp command
	kappCommand := kappcmd.NewDefaultKappCmd(confUI)

	// Set the command args with --json flag for structured output
	kappCommand.SetArgs([]string{
		"list",
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"--json",
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

	// Parse JSON output
	var listOutput KappListOutput
	if err := json.Unmarshal([]byte(outBuf.String()), &listOutput); err != nil {
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
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()

	// Create the kapp command
	kappCommand := kappcmd.NewDefaultKappCmd(confUI)

	// Build the full command args
	baseArgs := []string{
		"inspect",
		"-a", appName,
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"--color=false",
		"--tty=false",
	}
	kappCommand.SetArgs(append(baseArgs, flags...))

	// Capture output
	kappCommand.SetOut(&outBuf)
	kappCommand.SetErr(&errBuf)

	// Execute the command
	if err := kappCommand.Execute(); err != nil {
		return "", fmt.Errorf("kapp inspect failed: %w\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}

	return outBuf.String(), nil
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
