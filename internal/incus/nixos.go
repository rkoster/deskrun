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
		importLine := "  imports = [ ./deskrun.nix ];"
		if strings.Contains(configContent, "imports") {
			lines := strings.Split(configContent, "\n")
			var newLines []string
			for _, line := range lines {
				newLines = append(newLines, line)
				if strings.Contains(line, "imports") && strings.Contains(line, "[") {
					newLines = append(newLines, "    ./deskrun.nix")
				}
			}
			configContent = strings.Join(newLines, "\n")
		} else {
			configContent = strings.Replace(configContent, "{", "{\n"+importLine, 1)
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
