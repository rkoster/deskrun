package matchers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

// YAMLMatcher implements a Gomega matcher for YAML content with diff accept functionality
type YAMLMatcher struct {
	expectedFile string
	acceptMode   bool
	actualStr    string
	expectedStr  string
}

// MatchYAMLFile creates a new YAML matcher that compares against a file
// Set ACCEPT_DIFF=true or UPDATE_SNAPSHOTS=true to update expected files instead of comparing
func MatchYAMLFile(expectedFilePath string) types.GomegaMatcher {
	acceptMode := os.Getenv("ACCEPT_DIFF") == "true" ||
		os.Getenv("UPDATE_SNAPSHOTS") == "true"

	return &YAMLMatcher{
		expectedFile: expectedFilePath,
		acceptMode:   acceptMode,
	}
}

// Match performs the YAML comparison or accepts changes in accept mode
func (m *YAMLMatcher) Match(actual interface{}) (success bool, err error) {
	m.actualStr = toString(actual)

	if m.acceptMode {
		return m.acceptChanges()
	}

	return m.compareYAML()
}

// acceptChanges writes the actual content to the expected file
func (m *YAMLMatcher) acceptChanges() (bool, error) {
	// Ensure the directory exists
	dir := filepath.Dir(m.expectedFile)
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Write the actual content to expected file
	if err := os.WriteFile(m.expectedFile, []byte(m.actualStr), 0644); err != nil {
		return false, fmt.Errorf("failed to write expected file %s: %w", m.expectedFile, err)
	}

	return true, nil // Always pass in accept mode
}

// compareYAML performs the actual YAML comparison
func (m *YAMLMatcher) compareYAML() (bool, error) {
	// Read expected file content
	expectedBytes, err := os.ReadFile(m.expectedFile)
	if err != nil {
		return false, fmt.Errorf("failed to read expected file %s: %w", m.expectedFile, err)
	}
	m.expectedStr = string(expectedBytes)

	// For now, do a simple string comparison
	// TODO: Implement proper YAML diffing with dyff
	return strings.TrimSpace(m.actualStr) == strings.TrimSpace(m.expectedStr), nil
}

// FailureMessage returns the failure message when comparison fails
func (m *YAMLMatcher) FailureMessage(actual interface{}) string {
	if m.acceptMode {
		return "Accept mode is enabled but there was an error accepting changes"
	}

	return fmt.Sprintf(
		"Expected YAML to match file %s\n\n%s\n\nActual YAML:\n%s\n\nExpected YAML:\n%s",
		m.expectedFile,
		m.generateDiffReport(),
		format.IndentString(m.actualStr, 1),
		format.IndentString(m.expectedStr, 1),
	)
}

// NegatedFailureMessage returns the failure message for negated comparison
func (m *YAMLMatcher) NegatedFailureMessage(actual interface{}) string {
	if m.acceptMode {
		return "Accept mode is enabled - negated matches not supported"
	}

	return fmt.Sprintf(
		"Expected YAML NOT to match file %s, but they were identical",
		m.expectedFile,
	)
}

// generateDiffReport creates a simple diff report
func (m *YAMLMatcher) generateDiffReport() string {
	if strings.TrimSpace(m.actualStr) == strings.TrimSpace(m.expectedStr) {
		return "No differences found"
	}

	return "YAML content differs (TODO: implement proper YAML diffing)"
}

// toString converts various input types to string
func toString(input interface{}) string {
	switch v := input.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
