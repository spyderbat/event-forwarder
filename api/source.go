// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package api

import (
	"context"
	"encoding/json"
	"io"

	"github.com/valyala/fastjson"
)

type RuntimeDetails struct {
	CloudInstanceID string   `json:"cloud_instance_id,omitempty"`
	IPAddresses     []string `json:"ip_addresses"`
	MACAddresses    []string `json:"mac_addresses"`
	Hostname        string   `json:"hostname"`
	Forwarder       string   `json:"forwarder,omitempty"`
}

type Source struct {
	UID            string         `json:"uid"`
	RuntimeDetails RuntimeDetails `json:"runtime_details"`
}

// AugmentRuntimeDetailsJSON takes JSON input, extracts the muid if there is one,
// and augments the JSON with runtime_details from the source, if available.
func (a *API) AugmentRuntimeDetailsJSON(record []byte) []byte {
	if len(record) < 2 {
		return record
	}

	var details *RuntimeDetails

	if muid := fastjson.GetString(record, "muid"); muid != "" {
		if d, found := a.muid.Load(muid); found {
			details = &d
		}
	}

	if details == nil {
		details = &RuntimeDetails{}
	}

	details.Forwarder = a.useragent

	d, err := json.Marshal(details)
	if err != nil {
		return record
	}

	// This is not pretty, but it avoids the cost of parsing the log record.
	// We can't use append() anywhere, because we cannot modify the original record
	// without corrupting the underlying scanner.
	const key = `,"runtime_details":`
	newlen := len(record) + len(key) + len(d)
	newRecord := make([]byte, newlen)
	copy(newRecord, record[:len(record)-1])
	copy(newRecord[len(record)-1:], key)
	copy(newRecord[len(record)-1+len(key):], d)
	newRecord[len(newRecord)-1] = '}'
	return newRecord
}

// RefreshSources queries all sources from the API and populates the runtime details into an
// atomic map.
func (a *API) RefreshSources(ctx context.Context) error {
	sourcesJson, err := a.SourceQuery(ctx)
	if err != nil {
		return err
	}

	srcs, err := io.ReadAll(sourcesJson)
	sourcesJson.Close()
	if err != nil {
		return err
	}

	var source []Source
	err = json.Unmarshal(srcs, &source)
	if err != nil {
		return err
	}

	for _, v := range source {
		if len(v.UID) > 0 {
			a.muid.Store(v.UID, v.RuntimeDetails)
		}
	}

	return nil
}
