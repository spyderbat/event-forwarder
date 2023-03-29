package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHeader(t *testing.T) {
	resp := &http.Response{
		Header: map[string][]string{
			"X-Test": {"test"},
		},
	}

	assert.Equal(t, "test", getHeader(resp, "X-Test"))
	assert.Equal(t, "test", getHeader(resp, "x-test"))
	assert.Equal(t, "test", getHeader(resp, "X-test"))
	assert.Equal(t, "test", getHeader(resp, "x-Test"))
	assert.Equal(t, "", getHeader(resp, "Kittens"))
	assert.Equal(t, "", getHeader(nil, "Puppies"))
}

func TestAPIError(t *testing.T) {
	resp := &http.Response{
		Status:     "Forbidden",
		StatusCode: 403,
		Header: map[string][]string{
			"x-context-uid":    {"1234"},
			"x-jwt-expiration": {"2021-01-01T00:00:00Z"},
			"x-server-time":    {"2021-01-01T00:00:00Z"},
			"SeRvEr":           {"yes"},
		},
	}

	e := newAPIError(resp)

	assert.Equal(t, "Forbidden; spyderbat support id 1234; expiration 2021-01-01T00:00:00Z; server time 2021-01-01T00:00:00Z; check your host clock, your org uid, and your api key; server yes", e.Error())
}
