package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// runPassCmd runs a shell command and returns its stdout, with trailing
// newlines stripped, as a secret. The command is interpreted by `sh -c`, so it
// may be a pipeline — e.g. `security find-generic-password -a me -s qcal -w`.
func runPassCmd(cmd string) (string, error) {
	c := exec.Command("sh", "-c", cmd)
	var stderr bytes.Buffer
	c.Stderr = &stderr

	out, err := c.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("passcmd failed: %w: %s", err, msg)
		}
		return "", fmt.Errorf("passcmd failed: %w", err)
	}

	secret := strings.TrimRight(string(out), "\r\n")
	if secret == "" {
		return "", fmt.Errorf("passcmd returned an empty value")
	}
	return secret, nil
}
