// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log"
	"log/syslog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"spyderbat-event-forwarder/api"
	"spyderbat-event-forwarder/config"
	_ "spyderbat-event-forwarder/logwrapper"
	"spyderbat-event-forwarder/webhook"

	jsoniter "github.com/json-iterator/go"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

const (
	requestDelay      = 30 * time.Second // how long to wait between requests while in steady state
	recordsPerRequest = 10000
	noisy             = false
)

func printVersion() {
	vcsrevision := ""
	vcsdirty := ""
	vcstime := "unknown"
	version := "go1.x"

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, v := range info.Settings {
			switch v.Key {
			case "vcs.revision":
				vcsrevision = v.Value
			case "vcs.modified":
				if v.Value == "true" {
					vcsdirty = "+dirty"
				}
			case "vcs.time":
				vcstime = v.Value
			}
		}
		version = info.GoVersion
	}

	if len(vcsrevision) < 7 {
		vcsrevision = "unknown"
	}

	vcsrevision = vcsrevision[:7] // short hash

	log.Printf("starting spyderbat-event-forwarder/v2 (commit %s%s; %s; %s; %s)", vcsrevision, vcsdirty, vcstime, version, runtime.GOARCH)
}

func getUserAgent() string {
	vcsrevision := ""
	vcsdirty := ""

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, v := range info.Settings {
			switch v.Key {
			case "vcs.revision":
				vcsrevision = v.Value
			case "vcs.modified":
				if v.Value == "true" {
					vcsdirty = "+dirty"
				}
			}
		}
	}

	if len(vcsrevision) < 7 {
		vcsrevision = "unknown"
	}

	// the short hash is 7 characters, which also works with "unknown"
	return "sef/" + vcsrevision[:7] + vcsdirty
}

// pedantically use the same logic as the go runtime to get the proxy settings
func getEnvAny(names ...string) string {
	for _, n := range names {
		if val := os.Getenv(n); val != "" {
			return val
		}
	}
	return ""
}

func main() {
	configPath := flag.String("c", "config.yaml", "path to config file")
	flag.Parse()

	printVersion()
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("fatal: %s", err)
	}

	log.Printf("org uid: %s", cfg.OrgUID)
	log.Printf("api host: %s", cfg.APIHost)
	log.Printf("log path: %s", cfg.LogPath)
	log.Printf("local syslog forwarding: %v", cfg.LocalSyslogForwarding)

	if v := getEnvAny("HTTP_PROXY", "http_proxy"); v != "" {
		log.Printf("http proxy: %s", v)
	}
	if v := getEnvAny("HTTPS_PROXY", "https_proxy"); v != "" {
		log.Printf("https proxy: %s", v)
	}
	if v := getEnvAny("NO_PROXY", "no_proxy"); v != "" {
		log.Printf("no proxy: %s", v)
	}

	// resolve the API host to an IP address, just to verify DNS works
	addr, err := net.ResolveIPAddr("ip", cfg.APIHost)
	if err != nil {
		log.Printf("WARNING: dns check failed, unable to resolve %s: %s", cfg.APIHost, err)
		// not necessarily fatal if a proxy is involved
	} else {
		log.Printf("dns check successful: %s -> %s", cfg.APIHost, addr)
	}

	if cfg.Webhook != nil {
		log.Printf("webhook endpoint: %s", cfg.Webhook.Endpoint)
		log.Printf("webhook max payload bytes: %d", cfg.Webhook.MaxPayloadBytes)
		log.Printf("webhook ignore cert validation: %v", cfg.Webhook.Insecure)
		if cfg.Webhook.Authentication.Method != "" {
			log.Printf("webhook authentication method: %s", cfg.Webhook.Authentication.Method)
		}
		log.Printf("webhook compression algorithm: %s", cfg.Webhook.CompressionAlgo)
	} else {
		log.Printf("webhook: disabled")
	}

	sapi := api.New(cfg, getUserAgent())
	sapi.SetDebug(noisy)
	err = sapi.ValidateAPIReachability(context.Background())
	if err != nil {
		log.Fatalf("fatal: unable to reach API server: %s", err)
	} else {
		log.Printf("api server reachable")
	}

	// create a self-rotating logger to write our events to
	logWriters := []io.Writer{
		&lumberjack.Logger{
			Filename:   filepath.Join(cfg.LogPath, "spyderbat_events.log"),
			MaxSize:    10, // megabytes after which new file is created
			MaxBackups: 5,  // number of backups
		},
	}

	if cfg.StdOut {
		logWriters = append(logWriters, os.Stdout)
	}

	if cfg.LocalSyslogForwarding {
		w, err := syslog.Dial("", "", syslog.LOG_ALERT, "spyderbat-event")
		if err != nil {
			log.Printf("syslog forwarding requested, but failed: %s", err)
		} else {
			logWriters = append(logWriters, w)
		}
	}

	_ = sapi.RefreshSources(context.TODO())
	go func() {
		t := time.NewTicker(5 * time.Minute)
		for {
			<-t.C
			err := sapi.RefreshSources(context.Background())
			if err == nil {
				log.Printf("refreshed sources")
			} else {
				log.Printf("error refreshing sources: %s", err)
			}
		}
	}()

	eventLog := log.New(io.MultiWriter(logWriters...), "", 0)
	webhook := webhook.New(cfg.Webhook)

	// do a graceful shutdown on SIGTERM or SIGINT
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	ctx, cancel := context.WithCancel(context.Background())
	shutdownComplete := make(chan bool, 1)
	go func() {
		<-sig
		cancel()
		log.Printf("got shutdown signal, shutting down")
		webhook.Shutdown()
		shutdownComplete <- true
	}()

	iterator, err := cfg.GetIterator("OLDEST")
	if err != nil {
		log.Fatalf("fatal: unable to get current iterator: %s", err)
	}

	queryErrCount := 0
	records := 0
	const maxErrCount = 5

	req := &processLogsRequest{
		sapi:     sapi,
		eventLog: eventLog,
		stats:    new(logstats),
		webhook:  webhook,
	}

	buf := &bytes.Buffer{}

	delay := time.Second // Start log ingestion immediately
loop:
	for ctx.Err() == nil {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			break loop
		}

		if noisy {
			log.Printf("querying events from iterator=%s", iterator)
		}

		buf.Reset()
		records, iterator, err = sapi.LoadEvents(context.TODO(), iterator, recordsPerRequest, buf)
		if err != nil {
			delay = requestDelay
			queryErrCount++
			if queryErrCount > maxErrCount {
				// only log persistent errors; otherwise we'll just try again on the next loop iteration
				log.Printf("error querying events: %v", err)
			}
			continue
		}
		queryErrCount = 0
		if cfg.WriteIterator(iterator) != nil {
			log.Fatalf("fatal: unable to write iterator file: %s", err)
		}

		req.r = buf

		var now time.Time
		if noisy {
			now = time.Now()
			log.Printf("processing %d bytes ending in iterator %s", buf.Len(), iterator)
		}

		processLogs(ctx, req)

		if noisy {
			log.Printf("processed logs in %v", time.Since(now))
		}

		log.Printf("%d new records (%d invalid, %d logged)",
			req.stats.recordsRetrieved,
			req.stats.invalidRecords,
			req.stats.loggedRecords)

		if records >= recordsPerRequest {
			// if we got the number of records we requested, then we can assume that
			// more are available and we should immediately query again
			delay = 1 * time.Second
		} else {
			// if we got fewer records than we requested, then we should delay
			// before querying again, to avoid hammering the API
			delay = requestDelay
		}
	}
	<-shutdownComplete
	log.Printf("shutdown complete")
}
