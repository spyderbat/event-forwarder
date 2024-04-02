// Spyderbat Event Forwarder
// Copyright (C) 2022-2024 Spyderbat, Inc.
// Use according to license terms.

package main

import (
	"context"
	"flag"
	"fmt"
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
	"spyderbat-event-forwarder/lru"
	"spyderbat-event-forwarder/record"
	"spyderbat-event-forwarder/webhook"

	"github.com/expr-lang/expr/vm"
	jsoniter "github.com/json-iterator/go"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	json   = jsoniter.ConfigCompatibleWithStandardLibrary
	exprVM = vm.VM{} // exprVM is reused on each request
)

const (
	requestDelay    = 30 * time.Second // how long to wait between requests
	minQueryOverlap = 5 * time.Minute  // always look back at least this far
	maxQueryTime    = 15 * time.Minute // don't query more than this at once

	dedupCacheElements = 65536 * 10 // this ends up needing around a 4GB system

	noisy = false
)

// This is a simple lru cache to de-duplicate results from the backend,
// which will occur due to request window overlap and other reasons.
var lruCache *lru.LRU

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

	log.Printf("starting spyderbat-event-forwarder (commit %s%s; %s; %s; %s)", vcsrevision, vcsdirty, vcstime, version, runtime.GOARCH)
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

	lruCache, err = lru.New(dedupCacheElements, cfg.LogPath)
	if err != nil {
		log.Fatalf("fatal: unable to create or restore LRU cache: %s", err)
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

	// initial state: query from an hour ago until now
	st := cfg.GetCheckpoint(time.Now().Add(-1 * time.Hour))
	lastTime := record.RecordTimeFromTime(st)
	queryErrCount := 0
	processErrCount := 0
	const maxErrCount = 5

	req := &processLogsRequest{
		sapi:     sapi,
		eventLog: eventLog,
		stats:    new(logstats),
		lastTime: lastTime,
		cfg:      cfg,
		webhook:  webhook,
	}

	ticker := time.NewTicker(requestDelay)
	initialTick := make(chan bool, 1)
	initialTick <- true

loop:
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			break loop
		case <-ticker.C:
		case <-initialTick:
		}

		// query end time is always the current time
		et := time.Now()
		if et.Sub(st) > maxQueryTime {
			// if we're trying to query too much data, limit the query to maxQueryTime
			et = st.Add(maxQueryTime)
		}

		// If req.LastTime does not advance in the absence of errors, it will eventually
		// move forward due to maxQueryTime. This is useful for when the processing pipeline
		// is stalled for some reason.

		// if we have recent events, set the start time to the most recent event time
		if req.lastTime > 0 {
			if noisy {
				log.Printf("setting st from req.lastTime: %s", req.lastTime.Time())
			}
			st = req.lastTime.Time()
			cfg.WriteCheckpoint(st)
		}

		// If the start time is within 5 minutes of now, apply the minQueryOverlap.
		// This allows us to catch events that may be working through the system slowly.
		goBack := time.Now().Add(-minQueryOverlap)
		if goBack.Before(st) {
			if noisy {
				log.Printf("adjusting st for min query overlap: %s -> %s", st, goBack)
			}
			st = goBack
		}

		if noisy {
			log.Printf("querying source data from %s to %s (dur %v)", st, et, et.Sub(st))
		}
		r, err := sapi.SourceDataQuery(context.TODO(), st, et)
		if err != nil {
			queryErrCount++
			if queryErrCount > maxErrCount {
				// only log persistent errors; otherwise we'll just try again on the next loop iteration
				log.Printf("error querying source data: %v", err)
			}
			continue
		}
		queryErrCount = 0

		req.r = r

		var now time.Time
		if noisy {
			now = time.Now()
			log.Printf("processing logs from %s to %s (dur %v)", st, et, et.Sub(st))
		}
		err = processLogs(ctx, req)
		if err != nil {
			processErrCount++
			if processErrCount > maxErrCount {
				// only log persistent errors; otherwise we'll just try again on the next loop iteration
				log.Printf("error processing logs: %v", err)
			}
		} else {
			processErrCount = 0
		}
		// regardless of error, it's possible that some logs were processed.
		// Errors will be corrected on the next loop iteration.

		if noisy {
			log.Printf("processed logs in %v", time.Since(now))
		}

		lastStr := ""
		if req.stats.last > 0 {
			lastStr = fmt.Sprintf(", most recent %v ago", et.Sub(req.stats.last.Time()).Round(time.Second))
		}
		log.Printf("%d new records (%d invalid, %d logged, %d filtered)%s",
			req.stats.newRecords,
			req.stats.invalidRecords,
			req.stats.loggedRecords,
			req.stats.filteredRecords,
			lastStr)
	}
	<-shutdownComplete
	log.Printf("shutdown complete")
}
