// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

// logwrapper is a wrapper around the standard log package that forces all log output to be JSON
// and adds a schema and unique ID to each log entry.
package logwrapper

import (
	"crypto/rand"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

func _uid() string {
	const length = 11
	const corpus = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ01234567890"

	s := make([]byte, length*2)

	_, err := rand.Read(s[length:])
	if err != nil {
		panic(err)
	}

	for i := 0; i < length; i++ {
		s[i] = corpus[int(s[i+length])%len(corpus)]
	}

	return string(s[:length])
}

var (
	uid    = _uid()
	logger = setlogger()
)

type SchemaHook struct {
	sequence atomic.Int64
}

func (h *SchemaHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	e.Str("schema", "event_forwarder:meta:1.0.0")
	e.Str("id", fmt.Sprintf("event_meta:%s:%d", uid, h.sequence.Add(1)))
	e.Float64("time", float64(time.Now().UnixNano())/1e9)
}

func setlogger() *zerolog.Logger {
	l := zerolog.New(log.Writer()).With().Logger().Hook(&SchemaHook{})
	log.SetFlags(0)
	log.SetOutput(l)
	return &l
}

// Logger returns the global logger for packages that wish to use zerolog directly.
func Logger() *zerolog.Logger {
	return logger
}
