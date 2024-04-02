package journal

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

func TestJournal(t *testing.T) {
	var items []uint64
	dir := t.TempDir()

	t.Run("NewJournal", func(t *testing.T) {
		j, err := New(dir, 10, func(id uint64) {
			items = append(items, id)
		})
		require.NoError(t, err)
		require.NotNil(t, j)
		defer j.Close()
		require.Len(t, items, 0)

		for i := 0; i < 100; i++ {
			id := fmt.Sprintf("id-%d", i)
			idHash := blake2b.Sum256([]byte(id))
			err = j.Add(binary.LittleEndian.Uint64(idHash[:8]))
			require.NoError(t, err)
		}

		j.Close()

		t.Run("RestoreJournal", func(t *testing.T) {
			j, err := New(dir, 10, func(id uint64) {
				t.Logf("restoring %x", id)
				items = append(items, id)
			})
			require.NoError(t, err)
			require.NotNil(t, j)
			require.Len(t, items, 20)

			// validate the expected items are present
			for i := 80; i < 100; i++ {
				id := fmt.Sprintf("id-%d", i)
				idHash := blake2b.Sum256([]byte(id))
				require.Contains(t, items, binary.LittleEndian.Uint64(idHash[:8]))
			}

			// validate the unexpected items are not present
			for i := 0; i < 80; i++ {
				id := fmt.Sprintf("id-%d", i)
				idHash := blake2b.Sum256([]byte(id))
				require.NotContains(t, items, binary.LittleEndian.Uint64(idHash[:8]))
			}
		})
	})
}
