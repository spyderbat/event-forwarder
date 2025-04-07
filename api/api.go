// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"spyderbat-event-forwarder/config"

	"github.com/puzpuzpuz/xsync/v2"
)

const urlBase = "/api/v1/org/"

type APIError struct {
	StatusCode int
	Status     string
	Ctx        string
	Expiration string
	ServerTime string
	Server     string
}

func newAPIError(resp *http.Response) *APIError {
	return &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Ctx:        getHeader(resp, "X-Context-Uid"),
		Expiration: getHeader(resp, "X-Jwt-Expiration"),
		ServerTime: getHeader(resp, "X-Server-Time"),
		Server:     getHeader(resp, "Server"),
	}
}

// The API server does not currently use canonicalized HTTP headers, so we need to do a case-insensitive
// search for the header we want.
func getHeader(resp *http.Response, key string) string {
	if resp == nil {
		return ""
	}

	for k, v := range resp.Header {
		if strings.EqualFold(k, key) && len(v) > 0 {
			return v[0]
		}
	}

	return ""
}

func (e *APIError) Error() string {
	msg := e.Status
	if len(e.Ctx) > 0 {
		msg = fmt.Sprintf("%s; spyderbat support id %s", msg, e.Ctx)
	}
	if len(e.Expiration) > 0 {
		msg = fmt.Sprintf("%s; expiration %s", msg, e.Expiration)
	}
	if len(e.ServerTime) > 0 {
		msg = fmt.Sprintf("%s; server time %s", msg, e.ServerTime)
	}
	if e.StatusCode == http.StatusForbidden || e.StatusCode == http.StatusUnauthorized {
		msg = fmt.Sprintf("%s; check your host clock, your org uid, and your api key", msg)
	}
	if len(e.Server) > 0 {
		msg = fmt.Sprintf("%s; server %s", msg, e.Server)
	}
	return msg
}

type API struct {
	config    *config.Config
	client    *http.Client
	muid      *xsync.MapOf[string, RuntimeDetails]
	useragent string
	debug     bool
}

type APIer interface {
	AugmentRuntimeDetailsJSON(record []byte) []byte
	RefreshSources(ctx context.Context) error
}

// uaTransport wraps http.Transport to set a User-Agent header
type uaTransport struct {
	http.Transport
	UserAgent string
}

// RoundTrip wraps http.Transport.RoundTrip to set a User-Agent header
func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(t.UserAgent) > 0 {
		req.Header.Set("User-Agent", t.UserAgent)
	}
	return t.Transport.RoundTrip(req)
}

func New(c *config.Config, UserAgent string) *API {
	return &API{config: c, client: &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &uaTransport{
			Transport: http.Transport{
				Dial: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).Dial,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				DisableKeepAlives:     false,
				DisableCompression:    false,
				Proxy:                 http.ProxyFromEnvironment,
			},
			UserAgent: UserAgent,
		}},
		muid:      xsync.NewMapOf[RuntimeDetails](),
		useragent: UserAgent,
	}
}

// SetDebug enables or disables debug logging
func (a *API) SetDebug(d bool) {
	a.debug = d
}

func (a *API) ValidateAPIReachability(ctx context.Context) error {
	// GET /api/v1/org/{orgUID}
	url := fmt.Sprintf("https://%s%s%s", a.config.APIHost, urlBase, a.config.OrgUID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}

	resp.Body.Close()

	ae := newAPIError(resp)

	// if we have a context id, we know the API is reachable
	if len(ae.Ctx) > 0 {
		return nil
	}

	// otherwise, return diagnostic information
	return ae
}

// SourceQuery queries the API for sources
func (a *API) SourceQuery(ctx context.Context) (io.ReadCloser, error) {
	url := fmt.Sprintf("https://%s%s%s/source/", a.config.APIHost, urlBase, a.config.OrgUID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+a.config.APIKey)
	req.Header.Add("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}

	resp.Body.Close()

	return nil, newAPIError(resp)
}

type IteratorJSON struct {
	Iterator string `json:"iterator"`
}

// LoadEvents queries the API for events using the given iterator and writes the results to the given writer.
func (a *API) LoadEvents(ctx context.Context, iterator string, limit int, writeTo io.Writer) (records int, nextIterator string, anErr error) {
	url := fmt.Sprintf("https://%s%s%s/events/%s", a.config.APIHost, urlBase, a.config.OrgUID, iterator)
	if limit > 0 {
		url += fmt.Sprintf("?limit=%d", limit)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, iterator, err
	}
	req.Header.Add("Authorization", "Bearer "+a.config.APIKey)
	req.Header.Add("Accept", "application/x-ndjson, application/ndjson")
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, iterator, err
	}
	defer resp.Body.Close()

	if a.debug {
		ctxUid := getHeader(resp, "X-Context-Uid")
		log.Printf("context uid: %s", ctxUid)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, iterator, newAPIError(resp)
	}

	// Read a line at a time and copy it to writeTo
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		nextIter := IteratorJSON{}
		_ = json.Unmarshal(line, &nextIter)
		if len(nextIter.Iterator) > 0 {
			nextIterator = nextIter.Iterator
		} else {
			records++
			_, _ = writeTo.Write(line)
			_, _ = writeTo.Write([]byte("\n"))
		}
	}
	if err := scanner.Err(); err != nil {
		return records, iterator, err
	}
	return records, nextIterator, nil

}
