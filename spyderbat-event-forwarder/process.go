// Spyderbat Event Forwarder
// Copyright (C) 2022-2023 Spyderbat, Inc.
// Use according to license terms.

package main

import (
	"bufio"
	"context"
	"io"
	"log"
	"spyderbat-event-forwarder/api"
	"spyderbat-event-forwarder/config"
	"spyderbat-event-forwarder/record"
	"spyderbat-event-forwarder/webhook"
)

type logstats struct {
	recordsRetrieved int
	newRecords       int
	invalidRecords   int
	filteredRecords  int
	loggedRecords    int
	last             record.RecordTime
}

func (l *logstats) reset() {
	l.recordsRetrieved = 0
	l.newRecords = 0
	l.invalidRecords = 0
	l.filteredRecords = 0
	l.loggedRecords = 0
	// don't reset last
}

type processLogsRequest struct {
	r        io.ReadCloser
	sapi     api.APIer
	eventLog *log.Logger
	stats    *logstats
	lastTime record.RecordTime
	cfg      *config.Config
	webhook  *webhook.Webhook
}

func processLogs(ctx context.Context, req *processLogsRequest) error {
	req.stats.reset()
	defer req.r.Close()
	scanner := bufio.NewScanner(req.r)

	for scanner.Scan() && ctx.Err() == nil {
		req.stats.recordsRetrieved++
		jsonRecord := scanner.Bytes()

		// decode the record time
		id, t, err := record.SummaryFromJSON(jsonRecord)
		if err != nil {
			log.Printf("error decoding record: %s [%s]", err, string(jsonRecord))
			req.stats.invalidRecords++
			continue
		}

		// de-duplicate
		if _, exists := lruCache.Get(id); exists {
			//log.Printf("skipping duplicate: %s", id)
			continue
		} else {
			req.stats.newRecords++
			lruCache.Add(id, nil)
		}

		if t > req.lastTime {
			req.stats.last = t
		}

		emit := false

		if len(req.cfg.GetRegexes()) > 0 {
			for _, r := range req.cfg.GetRegexes() {
				if r.Match(jsonRecord) {
					emit = true
					break
				}
			}
		} else if req.cfg.GetExprProgram() != nil {
			// full JSON decoding is needed for expr ... this reduces performance ~90%
			var r map[string]any
			if err := json.Unmarshal(jsonRecord, &r); err != nil {
				panic(err) // the json was validated... this should never happen
			}
			out, err := exprVM.Run(req.cfg.GetExprProgram(), r)
			if err != nil {
				// if the expression is invalid, emit the event and carry on
				log.Printf("error evaluating expression: %s", err)
				emit = true
			} else {
				emit = out.(bool)
			}
		} else {
			emit = true
		}
		if emit {
			req.stats.loggedRecords++
			r := req.sapi.AugmentRuntimeDetailsJSON(jsonRecord)
			req.eventLog.Print(string(r))
			err = req.webhook.Send(r)
			if err != nil {
				log.Printf("error sending webhook: %s", err)
			}
		} else {
			req.stats.filteredRecords++
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("error processing records: %s", err)
	}

	return nil
}
