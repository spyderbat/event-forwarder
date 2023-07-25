package webhook

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"spyderbat-event-forwarder/config"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertBasicHeaders checks the basic request headers that should be present on every request
func assertBasicHeaders(t *testing.T, r *http.Request) {
	assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
	assert.Equal(t, "application/json", r.Header.Get("Accept"))
}

// TestWebhookSimple validates that a simple message is sent to the webhook.
func TestWebhookSimple(t *testing.T) {
	expectedBody := []byte(`{"foo":"bar"}`)
	visited := false

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, expectedBody, body)

		w.WriteHeader(http.StatusOK)
		visited = true
	}))

	cfg := &config.Webhook{
		Endpoint: ts.URL,
		Insecure: true,
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)

	err = h.Send(expectedBody)
	assert.NoError(t, err)

	h.Shutdown()
	ts.Close()
	assert.True(t, visited)
}

// TestWebhookSweep validates that messages are sent to the webhook after the sweep interval has elapsed.
func TestWebhookSweep(t *testing.T) {
	expectedBody := []byte(`{"foo":"bar"}`)
	visited := make(chan bool, 1)

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, expectedBody, body)

		w.WriteHeader(http.StatusOK)
		visited <- true
	}))

	// override the default values for testinga
	oldMaxPayloadAge := maxPayloadAge
	oldSweepInterval := sweepInterval
	t.Cleanup(func() {
		maxPayloadAge = oldMaxPayloadAge
		sweepInterval = oldSweepInterval
	})
	maxPayloadAge = 5 * time.Millisecond
	sweepInterval = 25 * time.Millisecond

	cfg := &config.Webhook{
		Endpoint: ts.URL,
		Insecure: true,
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)

	err = h.Send(expectedBody)
	assert.NoError(t, err)

	<-visited // the request should be sent before shutdown

	h.Shutdown()
	ts.Close()
}

// TestWebhookGzip validates that the gzip compression algorithm works correctly.
func TestWebhookGzip(t *testing.T) {
	visited := false
	expectedBody := []byte(`{"foo":"bar"}`)

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		if r.Header.Get("Content-Encoding") != "gzip" {
			t.Errorf("Expected Content-Encoding header to be gzip, but got %s", r.Header.Get("Content-Encoding"))
		}

		zipReader, err := gzip.NewReader(r.Body)
		require.NoError(t, err)

		body, err := io.ReadAll(zipReader)
		require.Equal(t, expectedBody, body)

		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		visited = true
	}))

	cfg := &config.Webhook{
		Endpoint:        ts.URL,
		Insecure:        true,
		CompressionAlgo: "gzip",
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)

	err = h.Send(expectedBody)
	assert.NoError(t, err)

	h.Shutdown()
	ts.Close()
	assert.True(t, visited)
}

// TestWebhookZstd validates that the Zstd compression algorithm works correctly.
func TestWebhookZstd(t *testing.T) {
	visited := false
	expectedBody := []byte(`{"foo":"bar"}`)

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		if r.Header.Get("Content-Encoding") != "zstd" {
			t.Errorf("Expected Content-Encoding header to be zstd, but got %s", r.Header.Get("Content-Encoding"))
		}

		zipReader, err := zstd.NewReader(r.Body)
		require.NoError(t, err)

		body, err := io.ReadAll(zipReader)
		require.Equal(t, expectedBody, body)

		require.NoError(t, err)

		w.WriteHeader(http.StatusOK)
		visited = true
	}))

	cfg := &config.Webhook{
		Endpoint:        ts.URL,
		Insecure:        true,
		CompressionAlgo: "zstd",
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)

	err = h.Send(expectedBody)
	assert.NoError(t, err)

	h.Shutdown()
	ts.Close()

	assert.True(t, visited)
}

// TestWebhookHMAC validates that the HMAC authentication method works correctly with an uncompressed payload.
func TestWebhookHMAC(t *testing.T) {
	visited := false
	expectedBody := []byte(`{"foo":"bar"}`)
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write(expectedBody)
	expectedMAC := mac.Sum(nil)

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, expectedBody, body)
		hexmac := r.Header.Get("X-HMAC")
		mac, err := hex.DecodeString(hexmac)
		assert.NoError(t, err)
		assert.True(t, hmac.Equal(expectedMAC, mac))

		w.WriteHeader(http.StatusOK)
		visited = true
	}))

	cfg := &config.Webhook{
		Endpoint:        ts.URL,
		MaxPayloadBytes: 262144,
		Insecure:        true,
		Authentication: config.WebhookAuthentication{
			Method: "hmac",
			Parameters: config.AuthenticationParameters{
				HeaderName:    "X-HMAC",
				SecretKey:     "dGVzdC1zZWNyZXQ=", // test-secret
				HashAlgorithm: "sha256",
			},
		},
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)

	err = h.Send(expectedBody)
	assert.NoError(t, err)

	h.Shutdown()
	ts.Close()
	assert.True(t, visited)
}

// TestWebhookZstdHMAC validates that the HMAC authentication method works correctly with Zstd compression.
func TestWebhookZstdHMAC(t *testing.T) {
	expectedPayload := []byte(`{"foo":"bar"}`)
	visited := false

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)
		require.Equal(t, "zstd", r.Header.Get("Content-Encoding"))

		zipReader, err := zstd.NewReader(r.Body)
		require.NoError(t, err)

		body, err := io.ReadAll(zipReader)
		assert.NoError(t, err)
		assert.Equal(t, expectedPayload, body)

		mac := hmac.New(sha256.New, []byte("test-secret"))
		mac.Write(body)
		expectedMAC := mac.Sum(nil)

		hexmac := r.Header.Get("X-HMAC")
		payloadMac, err := hex.DecodeString(hexmac)
		assert.NoError(t, err)
		assert.True(t, hmac.Equal(expectedMAC, payloadMac))

		w.WriteHeader(http.StatusOK)
		visited = true
	}))

	cfg := &config.Webhook{
		Endpoint:        ts.URL,
		Insecure:        true,
		CompressionAlgo: "zstd",
		Authentication: config.WebhookAuthentication{
			Method: "hmac",
			Parameters: config.AuthenticationParameters{
				HeaderName:    "X-HMAC",
				SecretKey:     "dGVzdC1zZWNyZXQ=", // test-secret
				HashAlgorithm: "sha256",
			},
		},
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)
	require.NotNil(t, h)
	err = h.Send(expectedPayload)
	require.NoError(t, err)

	h.Shutdown()
	ts.Close()

	assert.True(t, visited)
}

// TestWebhookLarge sends a large number of events to the webhook to ensure that the payload size limit is enforced
// and all events are sent.
func TestWebhookLarge(t *testing.T) {
	receiveBuffer := &bytes.Buffer{}

	var maxPayloadBytes int

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		n, err := io.Copy(receiveBuffer, r.Body)
		require.NoError(t, err)
		t.Logf("Received %d bytes", n)
		if n > int64(maxPayloadBytes) {
			t.Errorf("Received %d bytes, but max payload size is %d", n, maxPayloadBytes)
		}
		w.WriteHeader(http.StatusOK)
	}))

	cfg := &config.Webhook{
		Endpoint: ts.URL,
		Insecure: true,
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)
	maxPayloadBytes = h.c.MaxPayloadBytes

	msg := []byte(`{"foo":"bar"}`)
	bytesSent := 0

	for bytesSent < maxPayloadBytes*2 {
		err := h.Send(msg)
		assert.NoError(t, err)
		bytesSent += len(msg)
	}

	h.Shutdown()
	ts.Close()

	assert.Equal(t, bytesSent, receiveBuffer.Len())
}

// TestWebhookBasicAuth validates that the basic authentication method works correctly.
func TestWebhookBasicAuth(t *testing.T) {
	expectedBody := []byte(`{"foo":"bar"}`)
	visited := false

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		user, pass, ok := r.BasicAuth()
		require.True(t, ok)
		assert.Equal(t, "test-user", user)
		assert.Equal(t, "test-secret", pass)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, expectedBody, body)

		w.WriteHeader(http.StatusOK)
		visited = true
	}))

	cfg := &config.Webhook{
		Endpoint: ts.URL,
		Insecure: true,
		Authentication: config.WebhookAuthentication{
			Method: "basic",
			Parameters: config.AuthenticationParameters{
				Username: "test-user",
				Password: "dGVzdC1zZWNyZXQ=",
			},
		},
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)

	err = h.Send(expectedBody)
	assert.NoError(t, err)

	h.Shutdown()
	ts.Close()
	assert.True(t, visited)
}

// TestWebhookSharedSecret validates that the shared_secret authentication method works correctly.
func TestWebhookSharedSecret(t *testing.T) {
	expectedBody := []byte(`{"foo":"bar"}`)
	visited := false

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		assert.Equal(t, "test-secret", r.Header.Get("X-Shared-Secret"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, expectedBody, body)

		w.WriteHeader(http.StatusOK)
		visited = true
	}))

	cfg := &config.Webhook{
		Endpoint: ts.URL,
		Insecure: true,
		Authentication: config.WebhookAuthentication{
			Method: "shared_secret",
			Parameters: config.AuthenticationParameters{
				HeaderName: "X-Shared-Secret",
				SecretKey:  "dGVzdC1zZWNyZXQ=",
			},
		},
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)

	err = h.Send(expectedBody)
	assert.NoError(t, err)

	h.Shutdown()
	ts.Close()
	assert.True(t, visited)
}

// TestWebhookBearer validates that the bearer authentication method works correctly.
func TestWebhookBearer(t *testing.T) {
	expectedBody := []byte(`{"foo":"bar"}`)
	visited := false

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBasicHeaders(t, r)

		assert.Equal(t, "Bearer test-secret", r.Header.Get("Authorization"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, expectedBody, body)

		w.WriteHeader(http.StatusOK)
		visited = true
	}))

	cfg := &config.Webhook{
		Endpoint: ts.URL,
		Insecure: true,
		Authentication: config.WebhookAuthentication{
			Method: "bearer",
			Parameters: config.AuthenticationParameters{
				SecretKey: "dGVzdC1zZWNyZXQ=",
			},
		},
	}
	err := config.ValidateWebhook(cfg)
	require.NoError(t, err)
	h := New(cfg)

	err = h.Send(expectedBody)
	assert.NoError(t, err)

	h.Shutdown()
	ts.Close()
	assert.True(t, visited)
}

// TestNilSafe ensures that all webhook methods are nil-safe.
func TestNilSafe(t *testing.T) {
	h := New(nil)
	assert.Nil(t, h)

	err := h.Send([]byte(`{"foo":"bar"}`))
	assert.NoError(t, err)

	h.Shutdown()
}
