package api

import (
	"context"
	"fmt"
	"io"
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

// SourceDataQuery queries the API for events within a time range
func (a *API) SourceDataQuery(ctx context.Context, st time.Time, et time.Time) (io.ReadCloser, error) {
	url := fmt.Sprintf("https://%s%s%s/data?dt=redflags&st=%d&et=%d", a.config.APIHost, urlBase, a.config.OrgUID, st.Unix(), et.Unix())

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

	return nil, newAPIError(resp)
}
