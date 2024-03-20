// Spyderbat Event Forwarder
// Copyright (C) 2022-2024 Spyderbat, Inc.
// Use according to license terms.

package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckpoint(t *testing.T) {

	defaultTime, err := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
	require.NoError(t, err)

	c := Config{
		LogPath: t.TempDir(),
	}

	cp := c.GetCheckpoint(defaultTime)

	t.Run("NewCheckpoint", func(t *testing.T) {
		require.Equal(t, defaultTime, cp)
		_, err = os.Stat(c.checkpointFile())
		require.NoError(t, err)
	})

	t.Run("ExistingCheckpoint", func(t *testing.T) {
		cp = c.GetCheckpoint(time.Now())
		require.NotEqual(t, defaultTime.Round(time.Second), cp.Round(time.Second))
	})

	t.Run("WriteCheckpoint", func(t *testing.T) {
		now := time.Now()
		c.WriteCheckpoint(now)
		cp = c.GetCheckpoint(defaultTime)
		require.Equal(t, now.Round(time.Second), cp.Round(time.Second))
	})
}
