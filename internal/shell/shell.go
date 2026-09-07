// Package shell serves the generated git wrappers for supported shells.
package shell

import (
	"embed"
	"fmt"
)

//go:embed hooks/*
var hooks embed.FS

// Hook returns the shell function source for name.
func Hook(name string) (string, error) {
	file := map[string]string{
		"zsh":  "hooks/zsh.sh",
		"bash": "hooks/bash.sh",
		"fish": "hooks/fish.fish",
	}[name]
	if file == "" {
		return "", fmt.Errorf("no hook for %q; supported shells are zsh, bash and fish", name)
	}
	body, err := hooks.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
