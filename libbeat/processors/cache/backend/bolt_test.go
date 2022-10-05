package backend

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBolt(t *testing.T) {
	c, err := newBolt("mycache", &BoltConfig{
		Path: filepath.Join(t.TempDir(), "temp.db"),
	})
	require.NoError(t, err)
	defer c.close()

	err = c.Store("foo", "cached-value")
	require.NoError(t, err)

	value, err := c.Lookup("foo")
	require.NoError(t, err)
	t.Log(value)

	err = c.Delete("foo")
	require.NoError(t, err)
}
