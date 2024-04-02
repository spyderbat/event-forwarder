package lru

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLRU(t *testing.T) {
	dir := t.TempDir()

	t.Run("New", func(t *testing.T) {
		l, err := New(1_000_000, dir)
		require.NoError(t, err)
		require.NotNil(t, l)

		t.Run("BetterNotExist", func(t *testing.T) {
			for i := 0; i < 100; i++ {
				exists := l.Exists(fmt.Sprintf("id-%d", i))
				assert.False(t, exists)
			}
		})

		for i := 0; i < 100; i++ {
			err = l.Add(fmt.Sprintf("id-%d", i))
			require.NoError(t, err)
		}

		t.Run("Exists", func(t *testing.T) {
			for i := 0; i < 100; i++ {
				exists := l.Exists(fmt.Sprintf("id-%d", i))
				assert.True(t, exists)
			}
		})

		t.Run("Restore", func(t *testing.T) {
			l.close()
			l, err := New(1_000_000, dir)
			require.NoError(t, err)
			require.NotNil(t, l)

			t.Run("Exists", func(t *testing.T) {
				for i := 0; i < 100; i++ {
					exists := l.Exists(fmt.Sprintf("id-%d", i))
					require.True(t, exists)
				}
			})
		})
	})
}
