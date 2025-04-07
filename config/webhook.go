// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package config

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"io"
	"net/url"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const (
	defaultWebhookPayloadBytes = 1024 * 1024 * 1  // 1MB
	maxWebhookPayloadBytes     = 1024 * 1024 * 10 // 10MB
	minWebhookPayloadBytes     = 32 * 1024        // 32KB
)

type Webhook struct {
	Endpoint        string                `yaml:"endpoint_url"`
	Insecure        bool                  `yaml:"insecure"`
	CompressionAlgo string                `yaml:"compression_algo"`
	MaxPayloadBytes int                   `yaml:"max_payload_bytes"`
	Authentication  WebhookAuthentication `yaml:"authentication,omitempty"`
	compressor      func(io.Writer) Compressor
}

type WebhookAuthentication struct {
	Method     string                   `yaml:"method"`
	Parameters AuthenticationParameters `yaml:"parameters"`
}

type AuthenticationParameters struct {
	HeaderName    string `yaml:"header_name,omitempty"`
	SecretKey     string `yaml:"secret_key,omitempty"` // Base64 encoded
	HashAlgorithm string `yaml:"hash_algo,omitempty"`
	Username      string `yaml:"username,omitempty"`
	Password      string `yaml:"password,omitempty"` // Base64 encoded
	hasher        func() hash.Hash
}

func (a AuthenticationParameters) GetSecretKey() []byte {
	data, err := base64.StdEncoding.DecodeString(a.SecretKey)
	if err != nil {
		return nil
	}
	return data
}

func (a AuthenticationParameters) GetPassword() []byte {
	data, err := base64.StdEncoding.DecodeString(a.Password)
	if err != nil {
		return nil
	}
	return data
}

type Compressor io.WriteCloser

func (w *Webhook) Hasher() func() hash.Hash {
	return w.Authentication.Parameters.hasher
}

func (w *Webhook) Compressor() func(io.Writer) Compressor {
	return w.compressor
}

func ValidateWebhook(w *Webhook) error {
	if w == nil {
		return nil
	}
	if w.MaxPayloadBytes == 0 {
		w.MaxPayloadBytes = defaultWebhookPayloadBytes
	}
	if w.MaxPayloadBytes > maxWebhookPayloadBytes {
		return fmt.Errorf("webhook.max_payload_bytes cannot be greater than %d", maxWebhookPayloadBytes)
	}
	if w.MaxPayloadBytes < minWebhookPayloadBytes {
		return fmt.Errorf("webhook.max_payload_bytes cannot be less than %d", minWebhookPayloadBytes)
	}

	if w.Endpoint == "" {
		return fmt.Errorf("webhook.endpoint_url is required")
	}

	u, err := url.Parse(w.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse webhook.endpoint_url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("webhook.endpoint_url must use https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook.endpoint_url must include a hostname")
	}

	w.CompressionAlgo = strings.ToLower(w.CompressionAlgo)
	switch w.CompressionAlgo {
	case "gzip":
		w.compressor = func(w io.Writer) Compressor {
			c, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
			if err != nil {
				panic(err) // only panics if level is invalid
			}
			return c
		}
	case "zstd":
		w.compressor = func(w io.Writer) Compressor {
			c, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedFastest))
			if err != nil {
				panic(err) // only panics if level is invalid
			}
			return c
		}
	case "":
		w.CompressionAlgo = "none"
	case "none":
	default:
		return fmt.Errorf("unsupported compression algorithm '%s'", w.CompressionAlgo)
	}

	w.Authentication.Method = strings.ToLower(w.Authentication.Method)
	switch w.Authentication.Method {
	case "none":
	case "":
	case "basic":
		if w.Authentication.Parameters.Username == "" {
			return fmt.Errorf("webhook.authentication.username is required for basic auth")
		}
		if w.Authentication.Parameters.Password == "" {
			return fmt.Errorf("webhook.authentication.password is required for basic auth")
		}
		if w.Authentication.Parameters.GetPassword() == nil {
			return fmt.Errorf("webhook.authentication.password must be base64 encoded")
		}
	case "hmac":
		if w.Authentication.Parameters.HeaderName == "" {
			return fmt.Errorf("webhook.authentication.header_name is required for hmac auth")
		}
		if w.Authentication.Parameters.SecretKey == "" {
			return fmt.Errorf("webhook.authentication.secret_key is required for hmac auth")
		}
		if w.Authentication.Parameters.GetSecretKey() == nil {
			return fmt.Errorf("webhook.authentication.secret_key must be base64 encoded")
		}
		w.Authentication.Parameters.HashAlgorithm = strings.ToLower(w.Authentication.Parameters.HashAlgorithm)
		switch w.Authentication.Parameters.HashAlgorithm {
		case "sha256":
			w.Authentication.Parameters.hasher = sha256.New
		default:
			return fmt.Errorf("unsupported hash algorithm '%s'", w.Authentication.Parameters.HashAlgorithm)
		}
	case "bearer":
		if w.Authentication.Parameters.SecretKey == "" {
			return fmt.Errorf("webhook.authentication.secret_key is required for bearer auth")
		}
	case "shared_secret":
		if w.Authentication.Parameters.SecretKey == "" {
			return fmt.Errorf("webhook.authentication.secret_key is required for shared secret auth")
		}
		if w.Authentication.Parameters.HeaderName == "" {
			return fmt.Errorf("webhook.authentication.header_name is required for shared secret auth")
		}
	default:
		return fmt.Errorf("unsupported authentication method '%s'", w.Authentication.Method)
	}
	return nil
}
