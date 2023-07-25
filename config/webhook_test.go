package config

import (
	"testing"
)

func TestValidateWebhook(t *testing.T) {
	tests := []struct {
		name    string
		w       *Webhook
		wantErr bool
	}{
		{
			name: "valid webhook",
			w: &Webhook{
				Endpoint: "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "http webhook",
			w: &Webhook{
				Endpoint: "http://example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid max payload bytes",
			w: &Webhook{
				Endpoint:        "https://example.com",
				MaxPayloadBytes: maxWebhookPayloadBytes + 1,
			},
			wantErr: true,
		},
		{
			name: "minimum payload bytes",
			w: &Webhook{
				Endpoint:        "https://example.com",
				MaxPayloadBytes: minWebhookPayloadBytes - 1,
			},
			wantErr: true,
		},
		{
			name:    "missing endpoint",
			w:       &Webhook{},
			wantErr: true,
		},
		{
			name: "invalid endpoint url",
			w: &Webhook{
				Endpoint: string([]byte{0x7f}),
			},
			wantErr: true,
		},
		{
			name: "missing host in url",
			w: &Webhook{
				Endpoint: "https://",
			},
			wantErr: true,
		},
		{
			name: "unsupported authentication method",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "missing username for basic auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "basic",
					Parameters: AuthenticationParameters{
						Password: "password",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing password for basic auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "basic",
					Parameters: AuthenticationParameters{
						Username: "username",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid password for basic auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "basic",
					Parameters: AuthenticationParameters{
						Username: "username",
						Password: "pass%20word", // "password" is vaid base64!
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid password for basic auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "basic",
					Parameters: AuthenticationParameters{
						Username: "username",
						Password: "dGVzdC1zZWNyZXQ=",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing header name for hmac auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "hmac",
					Parameters: AuthenticationParameters{
						SecretKey: "secret",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing secret key for hmac auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "hmac",
					Parameters: AuthenticationParameters{
						HeaderName: "header",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "unsupported hash algorithm for hmac auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "hmac",
					Parameters: AuthenticationParameters{
						HeaderName:    "header",
						SecretKey:     "dGVzdC1zZWNyZXQ=",
						HashAlgorithm: "invalid",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "sha256 hash algorithm for hmac auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "hmac",
					Parameters: AuthenticationParameters{
						HeaderName:    "header",
						SecretKey:     "dGVzdC1zZWNyZXQ=",
						HashAlgorithm: "SHA256",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid secret for hmac auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "hmac",
					Parameters: AuthenticationParameters{
						HeaderName:    "header",
						SecretKey:     "secret",
						HashAlgorithm: "SHA256",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing secret key for bearer auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "bearer",
				},
			},
			wantErr: true,
		},
		{
			name: "missing secret key for shared secret auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "shared_secret",
					Parameters: AuthenticationParameters{
						HeaderName: "header",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing header name for shared secret auth",
			w: &Webhook{
				Endpoint: "https://example.com",
				Authentication: WebhookAuthentication{
					Method: "shared_secret",
					Parameters: AuthenticationParameters{
						SecretKey: "secret",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "gzip compression",
			w: &Webhook{
				Endpoint:        "https://example.com",
				CompressionAlgo: "gzip",
			},
			wantErr: false,
		},
		{
			name: "zstd compression",
			w: &Webhook{
				Endpoint:        "https://example.com",
				CompressionAlgo: "zstd",
			},
			wantErr: false,
		},
		{
			name: "none compression",
			w: &Webhook{
				Endpoint:        "https://example.com",
				CompressionAlgo: "none",
			},
			wantErr: false,
		},
		{
			name: "invalid compression",
			w: &Webhook{
				Endpoint:        "https://example.com",
				CompressionAlgo: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhook(tt.w)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWebhook() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
