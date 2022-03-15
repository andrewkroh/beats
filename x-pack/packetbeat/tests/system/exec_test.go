// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package system

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func execute(t testing.TB, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	packetbeatPath, err := filepath.Abs(Exe("../../packetbeat.test"))
	require.NoError(t, err)

	if _, err := os.Stat(packetbeatPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skipf("%v binary not found", filepath.Base(packetbeatPath))
		}
		t.Fatal(err)
	}

	stdoutBuf, stderrBuf := new(bytes.Buffer), new(bytes.Buffer)
	workdir := t.TempDir()

	testArgs := append(
		[]string{"-systemTest"},
		args...,
	)
	cmd := exec.Command(packetbeatPath, testArgs...)
	cmd.Dir = workdir
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	err = cmd.Run()

	return strings.TrimSpace(stdoutBuf.String()), strings.TrimSpace(stderrBuf.String()), err
}

func Exe(path string) string {
	if runtime.GOOS == "windows" {
		return path + ".exe"
	}
	return path
}
