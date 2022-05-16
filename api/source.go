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
}

type Source struct {
	UID            string         `json:"uid"`
	RuntimeDetails RuntimeDetails `json:"runtime_details"`
}

// AugmentRuntimeDetailsJSON takes JSON input, extracts the muid if there is one,
// and augments the JSON with runtime_details from the source, if available.
func (a *API) AugmentRuntimeDetailsJSON(record *[]byte) {
	if record == nil || len(*record) < 2 {
		return
	}

	muid := fastjson.GetString(*record, "muid")
	if muid == "" {
		return
	}

	details, found := a.muid.Load(muid)
	if !found {
		return
	}

	d, err := json.Marshal(details)
	if err != nil {
		return
	}

	// This is not pretty, but it avoids the cost of parsing the log record.
	*record = append(append((*record)[:len(*record)-1], append([]byte(`,"runtime_details":`), d...)...), '}')
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
