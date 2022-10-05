package cache

import (
	"github.com/elastic/elastic-agent-libs/logp"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBolt(t *testing.T) {
	c, err := newBolt(logp.NewLogger("javascript.cache"), BoltConfig{
		Path: filepath.Join(t.TempDir(), "temp.db"),
	})
	require.NoError(t, err)
	defer c.close()

	err = c.Put("foo", "cached-value")
	require.NoError(t, err)

	value, err := c.Get("foo")
	require.NoError(t, err)
	t.Log(value)

	err = c.Delete("foo")
	require.NoError(t, err)
}
