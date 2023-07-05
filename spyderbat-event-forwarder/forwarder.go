// Spyderbat Event Forwarder
// Copyright (C) 2022-2023 Spyderbat, Inc.
// Use according to license terms.

package main

import (
	"bufio"
	"context"
	"flag"
	"io"
	"io/fs"
	"log"
	"log/syslog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"spyderbat-event-forwarder/api"
	"spyderbat-event-forwarder/config"
	"spyderbat-event-forwarder/record"

	"github.com/antonmedv/expr/vm"
	"github.com/golang/groupcache/lru"
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

	dedupCacheElements = 65536 * 10 // this is likely about 8MB per 64k cache entries
)

// This is a simple lru cache to de-duplicate results from the backend,
// which will occur due to request window overlap and other reasons.
// The hash key is a hash of the log data. There is no value.
var lruCache = lru.New(dedupCacheElements)

// loadState seeds the LRU from events already written to disk. It returns the most recent event time.
func loadState(LogPath string) (record.RecordTime, error) {
	lastTime := record.RecordTime(0)
	LogPath = filepath.Clean(LogPath)
	err := filepath.WalkDir(LogPath, func(path string, d fs.DirEntry, err error) error {
		if d.Type().IsDir() && filepath.Dir(path) != filepath.Dir(LogPath) { // skip subdirs (except the root)
			return fs.SkipDir // don't descend into subdirs
		}
		if err != nil {
			return err
		}
		name := d.Name()
		if d.Type().IsRegular() && strings.HasPrefix(name, "spyderbat_events") && strings.HasSuffix(name, ".log") {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			cached := 0
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				b := scanner.Bytes()
				if id, t, err := record.SummaryFromJSON(b); err == nil {
					if t > lastTime {
						lastTime = t
					}
					lruCache.Add(id, nil)
					cached++
				}
			}
			if scanner.Err() != nil {
				return scanner.Err()
			}
			log.Printf("loaded %d IDs from %s", cached, path)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return lastTime, nil
}

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
					vcsdirty = " (dirty)"
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

	sapi := api.New(cfg, getUserAgent())
	err = sapi.ValidateAPIReachability(context.Background())
	if err != nil {
		log.Fatalf("fatal: unable to reach API server: %s", err)
	} else {
		log.Printf("api server reachable")
	}

	lastTime, err := loadState(cfg.LogPath)
	if err != nil {
		log.Printf("error loading state (ignored): %s", err)
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

	var last record.RecordTime

	// initial state: query from an hour ago until now
	st := time.Now().Add(-1 * time.Hour)

	startingUp := true

	req := &processLogsRequest{
		sapi:     sapi,
		eventLog: eventLog,
		last:     last,
		stats:    new(logstats),
		lastTime: lastTime,
		cfg:      cfg,
	}

	for {
		if !startingUp {
			time.Sleep(requestDelay)
		}
		startingUp = false

		// query end time is always the current time
		// todo: it might make sense to reduce this on startup to avoid requesting
		// too much data at once
		et := time.Now()

		// if we have recent events, set the start time to the most recent event time
		if lastTime > 0 {
			st = lastTime.Time()
		}

		// always look at least minQueryOverlap into the past
		maxStart := et.Add(-minQueryOverlap)
		if st.After(maxStart) {
			st = maxStart
		}

		// never look more than 4h into the past
		minStart := et.Add(-4 * time.Hour)
		if st.Before(minStart) {
			st = minStart
		}

		r, err := sapi.SourceDataQuery(context.TODO(), st, et)
		if err != nil {
			log.Printf("error querying source data: %v", err)
			continue
		}

		req.r = r

		err = processLogs(req)
		if err != nil {
			log.Printf("error processing logs: %v", err)
		}

		log.Printf("%d new records (%d invalid, %d logged, %d filtered), most recent %v ago",
			req.stats.newRecords,
			req.stats.invalidRecords,
			req.stats.loggedRecords,
			req.stats.filteredRecords,
			et.Sub(req.stats.last.Time()).Round(time.Second))
	}
}
