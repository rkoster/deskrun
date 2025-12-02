package kapp

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	kappcmd "carvel.dev/kapp/pkg/kapp/cmd"
	"github.com/cppforlife/go-cli-ui/ui"
	"gopkg.in/yaml.v3"
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
		"--json",
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

	// Parse JSON output to extract app names
	var result struct {
		Tables []struct {
			Rows []struct {
				Name string `yaml:"name"`
			} `yaml:"rows"`
		} `yaml:"tables"`
	}

	if err := yaml.Unmarshal(outBuf.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse kapp list output: %w", err)
	}

	var names []string
	if len(result.Tables) > 0 {
		for _, row := range result.Tables[0].Rows {
			names = append(names, row.Name)
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
