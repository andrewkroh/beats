// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

//go:build integration
// +build integration

package system

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevices(t *testing.T) {
	stdout, stderr, err := execute(t, "devices")
	require.NoError(t, err, stderr)
	t.Log("Output:\n", stdout)

	ifcs, err := net.Interfaces()
	require.NoError(t, err)

	for _, ifc := range ifcs {
		assert.Contains(t, stdout, ifc.Name)
	}
}
