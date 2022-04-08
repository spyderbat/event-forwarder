// Spyderbat Event Forwarder
// Copyright (C) 2022 Spyderbat, Inc.
// Use according to license terms.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"io"
	"io/fs"
	"log"
	"log/syslog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"spyderbat-event-forwarder/api"
	"spyderbat-event-forwarder/config"
	"spyderbat-event-forwarder/record"

	"github.com/golang/groupcache/lru"
	"golang.org/x/crypto/blake2b"
	"gopkg.in/natefinch/lumberjack.v2"
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
	record := new(record.Record)
	LogPath = filepath.Clean(LogPath)
	err := filepath.WalkDir(LogPath, func(path string, d fs.DirEntry, err error) error {
		if d.Type().IsDir() && d.Name() != LogPath {
			return fs.SkipDir // don't descend into subdirs
		}
		if err != nil {
			return err
		}
		name := d.Name()
		if d.Type().IsRegular() && strings.HasPrefix(name, "spyderbat_events") && strings.HasSuffix(name, ".log") {
			f, err := os.Open(name)
			if err != nil {
				return err
			}
			log.Printf("loading %s", name)
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				b := scanner.Bytes()
				if json.Unmarshal(b, record) == nil {
					if record.Time > lastTime {
						lastTime = record.Time
					}
				}
				lruCache.Add(blake2b.Sum256(b), nil)
			}
			if scanner.Err() != nil {
				return scanner.Err()
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return lastTime, nil
}

func printVersion() {
	vcsrevision := "unknown"
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

	log.Printf("starting spyderbat-event-forwarder (commit %s%s; %s; %s; %s)", vcsrevision, vcsdirty, vcstime, version, runtime.GOARCH)
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

	if cfg.LocalSyslogForwarding {
		w, err := syslog.Dial("", "", syslog.LOG_ALERT, "spyderbat-event")
		if err != nil {
			log.Printf("syslog forwarding requested, but failed: %s", err)
		} else {
			logWriters = append(logWriters, w)
		}
	}

	eventLog := log.New(io.MultiWriter(logWriters...), "", 0)

	sapi := api.New(cfg)

	// struct to decode log time from json records
	last := new(record.Record)

	// initial state: query from an hour ago until now
	st := time.Now().Add(-1 * time.Hour)

	startingUp := true

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

		scanner := bufio.NewScanner(r)
		recordsRetrieved := 0
		newRecords := 0

		for scanner.Scan() {
			recordsRetrieved++
			record := scanner.Bytes()
			sum := blake2b.Sum256(record)
			if _, exists := lruCache.Get(sum); exists {
				continue // skip duplicates
			} else {
				newRecords++
				lruCache.Add(sum, nil)
			}

			err := json.Unmarshal(record, last)
			if err != nil {
				continue
			}

			if last.Time > lastTime {
				lastTime = last.Time
			}

			eventLog.Print(string(record))
		}
		r.Close()
		if err := scanner.Err(); err != nil {
			log.Printf("error processing records: %s", err)
		}

		log.Printf("%d new records, most recent %v ago", newRecords, et.Sub(lastTime.Time()).Round(time.Second))
	}
}
