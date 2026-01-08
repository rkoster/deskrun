package incus

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed templates/deskrun.nix
var deskrunNixTemplate string

func (m *Manager) ConfigureNixOS(ctx context.Context, containerName string) error {
	if err := m.PushContent(ctx, containerName, deskrunNixTemplate, "/etc/nixos/deskrun.nix"); err != nil {
		return fmt.Errorf("failed to push deskrun.nix: %w", err)
	}

	configContent, err := m.Exec(ctx, containerName, "cat", "/etc/nixos/configuration.nix")
	if err != nil {
		return fmt.Errorf("failed to read configuration.nix: %w", err)
	}

	if !strings.Contains(configContent, "./deskrun.nix") {
		lines := strings.Split(configContent, "\n")
		var newLines []string
		foundImports := false
		insideImports := false
		importIndent := ""

		for i, line := range lines {
			newLines = append(newLines, line)

			if !foundImports && strings.Contains(line, "imports") {
				foundImports = true
				if strings.Contains(line, "[") {
					insideImports = true
					leadingSpaces := len(line) - len(strings.TrimLeft(line, " \t"))
					importIndent = strings.Repeat(" ", leadingSpaces+2)

					if strings.Contains(line, "];") || (strings.Contains(line, "]") && strings.Contains(line, ";")) {
						insideImports = false
					} else {
						continue
					}
				}
			} else if foundImports && insideImports {
				if !strings.HasPrefix(strings.TrimSpace(line), "./") &&
					!strings.HasPrefix(strings.TrimSpace(line), "<") &&
					!strings.HasPrefix(strings.TrimSpace(line), "#") {
					if strings.Contains(line, "]") {
						newLines = append(newLines[:len(newLines)-1], importIndent+"./deskrun.nix", line)
						insideImports = false
						foundImports = true
						continue
					}
				}
				continue
			}

			if foundImports && !insideImports && i+1 < len(lines) {
				nextLine := lines[i+1]
				if strings.HasPrefix(strings.TrimSpace(nextLine), "[") {
					insideImports = true
					leadingSpaces := len(line) - len(strings.TrimLeft(line, " \t"))
					importIndent = strings.Repeat(" ", leadingSpaces+2)
				}
			}
		}

		if !foundImports {
			importLine := "  imports = [ ./deskrun.nix ];"
			configContent = strings.Replace(configContent, "{", "{\n"+importLine, 1)
		} else {
			configContent = strings.Join(newLines, "\n")
		}

		if err := m.PushContent(ctx, containerName, configContent, "/etc/nixos/configuration.nix"); err != nil {
			return fmt.Errorf("failed to update configuration.nix: %w", err)
		}
	}

	fmt.Println("Running nixos-rebuild switch (this may take a few minutes)...")
	if _, err := m.Exec(ctx, containerName, "nixos-rebuild", "switch"); err != nil {
		return fmt.Errorf("failed to run nixos-rebuild switch: %w", err)
	}

	return nil
}
