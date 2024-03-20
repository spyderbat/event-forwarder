// Spyderbat Event Forwarder
// Copyright (C) 2022-2024 Spyderbat, Inc.
// Use according to license terms.

package lru

// This package is a simple journaling LRU cache to keep track of duplicate IDs.
// The journal is used to keep track of items being added to and fetched from the LRU.
// (Items are never deleted from the LRU except by eviction.)
//
// The purpose of this is to keep state, so when the application is restarted, it can
// pick up where it left off. This is important for the event forwarder, which is likely
// to encounter many duplicate IDs in the logs it processes.

import (
	"encoding/binary"
	"spyderbat-event-forwarder/lru/journal"

	hlru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/crypto/blake2b"
)

type lruValue struct{}

var noValue = lruValue{}

// LRU is a wrapper around an LRU cache that adds a journal.
type LRU struct {
	lru *hlru.Cache[uint64, lruValue]
	j   *journal.Journal
}

// New creates a new LRU.
func New(maxEntries int, dir string) (*LRU, error) {

	// IDs are blake2b hashes reduced to 64 bits
	l, err := hlru.New[uint64, lruValue](maxEntries)
	if err != nil {
		return nil, err
	}

	j, err := journal.New(dir, maxEntries*10, func(id uint64) {
		l.Add(id, noValue)
	})
	if err != nil {
		return nil, err
	}

	return &LRU{
		lru: l,
		j:   j,
	}, nil
}

// Add adds an item to the LRU.
func (l *LRU) Add(id string) error {
	idHash := blake2b.Sum256([]byte(id))
	key := binary.LittleEndian.Uint64(idHash[:8])
	l.lru.Add(key, noValue)
	return l.j.Add(key)
}

// Exists checks if an item exists in the LRU.
func (l *LRU) Exists(id string) bool {
	idHash := blake2b.Sum256([]byte(id))
	_, ok := l.lru.Get(binary.LittleEndian.Uint64(idHash[:8]))
	return ok
}

// close is unexported and for testing only.
func (l *LRU) close() error {
	return l.j.Close()
}
