package backend

import (
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestComponentRegistry(t *testing.T) {
	var reg ComponentRegistry

	err := reg.ConfigureComponents([]ComponentConfig{
		{
			ID: "mycache",
			Bolt: &BoltConfig{
				Path: filepath.Join(t.TempDir(), "test.db"),
			},
		},
	})
	require.NoError(t, err)
	defer reg.Close()

	backend, err := reg.Lookup("mycache")
	require.NoError(t, err)

	require.NotNil(t, backend)
}
