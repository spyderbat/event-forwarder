// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package api

import (
	"spyderbat-event-forwarder/config"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAugmentRuntimeDetails(t *testing.T) {
	record := []byte(`{"schema": "test_shenanigans:1.0.0","muid":"test","time":1642790400}`)
	origRecord := make([]byte, len(record))
	copy(origRecord, record)

	t.Logf("original record: %s", string(record))

	details := RuntimeDetails{
		CloudInstanceID: "kittens",
		IPAddresses:     []string{"256.256.256.256"},
		MACAddresses:    []string{"GG:GG:GG:GG:GG:GG"},
		Hostname:        "puppies",
	}

	jsonTarget := `
	{
		"schema": "test_shenanigans:1.0.0",
		"muid": "test",
		"time": 1642790400,
		"runtime_details": {
		  "cloud_instance_id": "kittens",
		  "ip_addresses": [	"256.256.256.256" ],
		  "mac_addresses": [ "GG:GG:GG:GG:GG:GG" ],
		  "hostname": "puppies",
		  "forwarder": "test/1.0"
		}
	}`

	a := New(&config.Config{}, "test/1.0")

	a.muid.Store("test", details)

	newRecord := a.AugmentRuntimeDetailsJSON(record)

	t.Logf("augmented record: %s", string(newRecord))

	require.JSONEq(t, jsonTarget, string(newRecord))

	// ensure the original record was not modified, which would cause corruption of the underlying scanner
	require.Equal(t, origRecord, record)
}
