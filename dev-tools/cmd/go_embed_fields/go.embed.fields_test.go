package main_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratedFields(t *testing.T) {
	// Generate files using the tool from this package.
	cmd := exec.Command("go", "generate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "test"
	require.NoError(t, cmd.Run())

	// Run the tests in target package to verify the generated files work.
	args := []string{"test"}
	if testing.Verbose() {
		args = append(args, "-v")
	}
	cmd = exec.Command("go", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "test"
	require.NoError(t, cmd.Run())
}
