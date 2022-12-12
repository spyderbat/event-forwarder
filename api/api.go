package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"spyderbat-event-forwarder/config"

	"github.com/puzpuzpuz/xsync"
)

const urlBase = "/api/v1/org/"

type APIError struct {
	StatusCode int
	Status     string
	Ctx        string
	Expiration string
	ServerTime string
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
		msg = fmt.Sprintf("%s; check your host clock, your org uid, and your api key.", msg)
	}
	return msg
}

type API struct {
	config *config.Config
	client *http.Client
	muid   *xsync.MapOf[RuntimeDetails]
}

func New(c *config.Config) *API {
	return &API{config: c, client: &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableKeepAlives:     false,
			DisableCompression:    false,
			Proxy:                 http.ProxyFromEnvironment,
		}},
		muid: xsync.NewMapOf[RuntimeDetails](),
	}
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

	return nil, &APIError{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Ctx:        resp.Header.Get("X-Context-Uid"),
		Expiration: resp.Header.Get("X-Jwt-Expiration"),
		ServerTime: resp.Header.Get("X-Server-Time"),
	}
}

// SourceDataQuery queries the API for events within a time range
func (a *API) SourceDataQuery(ctx context.Context, dataType string, st, et time.Time) (io.ReadCloser, error) {
	url := fmt.Sprintf("https://%s%s%s/data?dt=%s&st=%d&et=%d", a.config.APIHost, urlBase, a.config.OrgUID, dataType, st.Unix(), et.Unix())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+a.config.APIKey)
	req.Header.Add("Accept", "application/x-ndjson, application/ndjson")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}

	resp.Body.Close()

	return nil, &APIError{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Ctx:        resp.Header.Get("X-Context-Uid"),
		Expiration: resp.Header.Get("X-Jwt-Expiration"),
		ServerTime: resp.Header.Get("X-Server-Time"),
	}
}
