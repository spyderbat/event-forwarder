// Spyderbat Event Forwarder
// Copyright (C) 2022-2024 Spyderbat, Inc.
// Use according to license terms.

package journal

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
)

/* The journal consists of a directory containing at most two files with a sequence
 * of identifiers. Once the journal reaches the maximum number of entries, it is
 * rotated.
 *
 * The files are id_journal and id_journal.1. The first file is the current journal,
 * and the second file is the previous journal. When the current journal is rotated,
 * the previous journal is deleted, and the current journal is renamed to the previous
 * journal. A new current journal is created.
 */

const (
	filename = "id_journal"
	mode     = 0600
	flag     = os.O_APPEND | os.O_CREATE | os.O_WRONLY
)

type Journal struct {
	dir        string
	file       *os.File // The journal file is just a sequence of 64-bit hashed IDs
	entries    int
	maxEntries int
}

func (j *Journal) currentFile() string { return filepath.Join(j.dir, filename) }
func (j *Journal) backupFile() string  { return filepath.Join(j.dir, filename+".1") }

// replay replays the journal file, calling the rehydrate function for each entry.
// If the file does not exist, replay returns nil.
func replay(filePath string, rehydrate func(id uint64)) error {
	f, err := os.Open(filePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	defer f.Close()

	buf := make([]byte, 8)
	var id uint64

	for {
		_, err := io.ReadFull(f, buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			} else {
				return err
			}
		}
		id = binary.LittleEndian.Uint64(buf)
		rehydrate(id)
	}
}

// New creates opens an existing journal or creates a new one.
// The onAdd function is called for each entry in the journal.
func New(dir string, maxEntries int, onAdd func(id uint64)) (*Journal, error) {
	j := &Journal{
		dir:        dir,
		maxEntries: maxEntries,
	}

	// replay the files

	// start with the backup file
	err := replay(j.backupFile(), onAdd)
	if err != nil {
		return nil, err
	}

	// then replay the current file, keeping track of the number of entries
	err = replay(j.currentFile(), func(id uint64) {
		j.entries++
		onAdd(id)
	})
	if err != nil {
		return nil, err
	}

	// open the current file for writing
	j.file, err = os.OpenFile(j.currentFile(), flag, mode)
	if err != nil {
		return nil, err
	}

	return j, nil
}

// Add adds an entry to the journal.
func (j *Journal) Add(id uint64) error {
	if j.entries >= j.maxEntries {
		// rotate the journal
		err := j.rotate()
		if err != nil {
			return err
		}
	}
	be := make([]byte, 8)
	binary.LittleEndian.PutUint64(be, id)
	_, err := j.file.Write(be)
	if err != nil {
		return err
	}

	j.entries++
	return nil
}

// rotate the journal. This is called when the journal is full.
func (j *Journal) rotate() error {
	j.file.Close()
	err := os.Rename(j.currentFile(), j.backupFile())
	if err != nil {
		return err
	}
	j.file, err = os.OpenFile(j.currentFile(), flag, mode)
	if err != nil {
		return err
	}
	j.entries = 0
	return nil
}

// Close closes the journal.
func (j *Journal) Close() error {
	err := j.file.Close()
	if err != nil {
		return err
	}
	j.file = nil // subsequent calls to Add will panic
	return nil
}
