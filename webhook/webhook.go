// Spyderbat Event Forwarder
// Copyright (C) 2023 Spyderbat, Inc.
// Use according to license terms.

package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/tls"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"net"
	"net/http"
	"spyderbat-event-forwarder/config"
	"spyderbat-event-forwarder/logwrapper"
	"sync"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

var (
	maxPayloadAge = 30 * time.Second // how often to flush the queue
	sweepInterval = 1 * time.Second  // how often to check if the current buffer is old enough to be flushed
)

var (
	// ErrEmptyPayload is returned when an empty payload is sent to the webhook
	ErrEmptyPayload = errors.New("empty payload")
)

type httpclient interface {
	Do(req *retryablehttp.Request) (*http.Response, error)
}

type Webhook struct {
	c      *config.Webhook
	ctx    context.Context // ctx is used to shut down the webhook
	cancel context.CancelFunc
	wg     sync.WaitGroup // wg is used to wait for shutdown
	client httpclient

	messageQueue chan []byte   // messages are queued here before being written to the payload
	payloadQueue chan *payload // payloads are queued here before being sent to the webhook

	// the following fields are only accessed by the run goroutine
	created time.Time     // created is the time the current buffer was created
	count   int           // count is the number of events written to the buffer
	payload *bytes.Buffer // payload is the current payload buffer
}

type payload struct {
	bytes []byte
	count int
}

// New creates a new Webhook instance from the given config. If the config is nil, nil is returned.
// A nil Webhook will silently drop all events. To ensure that all events are sent, use Shutdown to
// flush the queue before exiting.
func New(c *config.Webhook) *Webhook {
	if c == nil {
		return nil
	}

	client := retryablehttp.NewClient()
	client.Logger = nil
	client.RetryMax = 5
	client.HTTPClient.Timeout = 2 * time.Minute
	client.HTTPClient.Transport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.Insecure,
		},
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    true,
		Proxy:                 http.ProxyFromEnvironment,
	}

	ctx, cancel := context.WithCancel(context.Background())
	h := &Webhook{
		c:            c,
		ctx:          ctx,
		cancel:       cancel,
		messageQueue: make(chan []byte, 10000),
		payloadQueue: make(chan *payload, 10), // with 1MB payloads, this is 10MB of memory
		client:       client,
	}
	h.resetPayload()
	h.wg.Add(2)
	go h.ingest()
	go h.sender()
	return h
}

func (h *Webhook) queuePayload() {
	defer h.resetPayload()
	if h.payload.Len() == 0 {
		return
	}
	h.payloadQueue <- &payload{
		bytes: h.payload.Bytes(),
		count: h.count,
	}
}

func (h *Webhook) resetPayload() {
	h.payload = &bytes.Buffer{}
	h.count = 0
	h.created = time.Now()
}

// sender is the main loop for the webhook sender.
// It will exit when the payloadQueue is closed.
func (h *Webhook) sender() {
	defer h.wg.Done()

	for payload := range h.payloadQueue {
		if payload == nil {
			return
		}
		err := h.send(payload)
		if err != nil {
			logwrapper.Logger().Error().Err(err).Msg("Failed to send event to webhook")
		}
	}
}

// ingest is the main loop for the webhook.
func (h *Webhook) ingest() {
	ticker := time.NewTicker(sweepInterval)

	defer func() {
		ticker.Stop()
		h.wg.Done()
	}()

	for {
		select {
		case <-ticker.C:
			// if the current buffer is older than maxPayloadAge, queue it for sending
			if time.Since(h.created) > maxPayloadAge {
				h.queuePayload()
			}
		case msg := <-h.messageQueue:
			// if the message would exceed the maximum payload size, queue it for sending
			if h.payload.Len()+len(msg) > h.c.MaxPayloadBytes {
				h.queuePayload()
			}
			// write the message to the current buffer
			// a write to a bytes.Buffer never returns an error
			_, _ = h.payload.Write(msg)
			h.count++
		case <-h.ctx.Done():
			// We must drain the message queue before shutting down, or we have a race condition.
			// By closing the message queue here, we ensure that the following range loop will
			// not block forever.
			close(h.messageQueue)

			for msg := range h.messageQueue {
				if msg == nil {
					break
				}
				// if the message would exceed the maximum payload size, queue it for sending
				if h.payload.Len()+len(msg) > h.c.MaxPayloadBytes {
					h.queuePayload()
				}
				// write the message to the current buffer
				// a write to a bytes.Buffer never returns an error
				_, _ = h.payload.Write(msg)
				h.count++
			}

			h.queuePayload()
			close(h.payloadQueue)
			return
		}
	}
}

type WebhookError struct {
	StatusCode      int
	Body            string
	RequestHeaders  http.Header
	ResponseHeaders http.Header
}

func (e *WebhookError) Error() string {
	return fmt.Sprintf("webhook returned status code %d; request headers %+#v; response headers %+#v; body %s",
		e.StatusCode, e.RequestHeaders, e.ResponseHeaders, e.Body)
}

func NewWebhookError(req *http.Request, resp *http.Response, body string) *WebhookError {
	return &WebhookError{
		StatusCode:      resp.StatusCode,
		Body:            body,
		RequestHeaders:  req.Header,
		ResponseHeaders: resp.Header,
	}
}

// send sends the given payload to the webhook. It will return an error if the webhook returns a non-2xx status code.
// It does not acquire a lock, and the lock need not be held.
func (h *Webhook) send(p *payload) error {

	body := &bytes.Buffer{}
	var writer io.Writer
	var closer io.Closer
	var pHMAC hash.Hash
	writer = body

	// Enable compression if configured
	if newZipper := h.c.Compressor(); newZipper != nil {
		z := newZipper(writer)
		writer = z
		closer = z
	}

	// Use a multiwriter if HMAC is enabled. HMAC is computed on the uncompressed data.
	if hasher := h.c.Hasher(); hasher != nil {
		pHMAC = hmac.New(hasher, h.c.Authentication.Parameters.GetSecretKey())
		writer = io.MultiWriter(writer, pHMAC)
	}

	_, err := writer.Write(p.bytes)
	if err != nil {
		return err
	}
	if closer != nil {
		err = closer.Close()
		if err != nil {
			return err
		}
	}

	req, err := retryablehttp.NewRequest(http.MethodPost, h.c.Endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if h.c.Authentication.Method == "basic" {
		req.SetBasicAuth(h.c.Authentication.Parameters.Username, string(h.c.Authentication.Parameters.GetPassword()))
	} else if h.c.Authentication.Method == "shared_secret" {
		req.Header.Set(h.c.Authentication.Parameters.HeaderName, string(h.c.Authentication.Parameters.GetSecretKey()))
	} else if h.c.Authentication.Method == "bearer" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.c.Authentication.Parameters.GetSecretKey()))
	} else if pHMAC != nil {
		req.Header.Set(h.c.Authentication.Parameters.HeaderName, fmt.Sprintf("%x", pHMAC.Sum(nil)))
	}
	if h.c.CompressionAlgo == "zstd" {
		req.Header.Set("Content-Encoding", "zstd")
	} else if h.c.CompressionAlgo == "gzip" {
		req.Header.Set("Content-Encoding", "gzip")
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	resp.Body.Close()
	if err != nil {
		return err
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logwrapper.Logger().Info().
			Int("events", p.count).
			Int("bytes", len(p.bytes)).
			Int("compressed_bytes", body.Len()).
			Int("status_code", resp.StatusCode).
			Msg("published to webhook")
		return nil
	}

	return NewWebhookError(req.Request, resp, string(respBody))
}

// Send queues an event for sending to the webhook. It will be sent asynchronously.
// Calling Send after Shutdown will panic.
func (h *Webhook) Send(event []byte) error {
	if h == nil {
		return nil
	}
	if len(event) == 0 {
		return ErrEmptyPayload
	}

	h.messageQueue <- event
	return nil
}

// Shutdown flushes the queue and shuts down the webhook. It will block until the queue is empty.
func (h *Webhook) Shutdown() {
	log.Printf("shutting down webhook")
	if h == nil {
		return
	}
	h.cancel()
	h.wg.Wait()
}
