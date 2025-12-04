package kapp

import (
	"bytes"
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

// Inspect inspects a kapp app
func (c *Client) Inspect(appName string) (string, error) {
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()

	// Create the kapp command
	kappCommand := kappcmd.NewDefaultKappCmd(confUI)

	// Set the command args
	kappCommand.SetArgs([]string{
		"inspect",
		"-a", appName,
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"--json",
	})

	// Capture output
	kappCommand.SetOut(&outBuf)
	kappCommand.SetErr(&errBuf)

	// Execute the command
	if err := kappCommand.Execute(); err != nil {
		return "", fmt.Errorf("kapp inspect failed: %w\nstderr: %s", err, errBuf.String())
	}

	return outBuf.String(), nil
}

// InspectWithTree inspects a kapp app with tree output showing resource hierarchy
func (c *Client) InspectWithTree(appName string) (string, error) {
	// Create a buffer to capture output
	var outBuf, errBuf bytes.Buffer
	confUI := ui.NewConfUI(ui.NewNoopLogger())
	confUI.EnableNonInteractive()

	// Create the kapp command
	kappCommand := kappcmd.NewDefaultKappCmd(confUI)

	// Set the command args
	kappCommand.SetArgs([]string{
		"inspect",
		"-a", appName,
		"--kubeconfig-context", c.kubeconfig,
		"-n", c.namespace,
		"--tree",
	})

	// Capture output
	kappCommand.SetOut(&outBuf)
	kappCommand.SetErr(&errBuf)

	// Execute the command
	if err := kappCommand.Execute(); err != nil {
		return "", fmt.Errorf("kapp inspect failed: %w\nstderr: %s", err, errBuf.String())
	}

	return outBuf.String(), nil
}
