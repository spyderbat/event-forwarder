// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package config

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIterator(t *testing.T) {
	defaultIterator, err := uuid.NewRandom()
	require.NoError(t, err)

	c := Config{
		LogPath: t.TempDir(),
	}

	cp, err := c.GetIterator(defaultIterator.String())
	require.NoError(t, err)

	t.Run("NewIterator", func(t *testing.T) {
		require.Equal(t, defaultIterator.String(), cp)
	})
	newIterator, err := uuid.NewRandom()
	require.NoError(t, err)
	err = c.WriteIterator(newIterator.String())
	require.NoError(t, err)
	cp, err = c.GetIterator(defaultIterator.String())
	require.NoError(t, err)
	require.Equal(t, newIterator.String(), cp)
	t.Run("ExistingIterator", func(t *testing.T) {
		newDefault := "abcd1234"
		cp, err = c.GetIterator(newDefault)
		require.NoError(t, err)
		require.Equal(t, newIterator.String(), cp)
	})
}
