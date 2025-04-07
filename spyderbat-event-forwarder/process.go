// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package main

import (
	"bufio"
	"context"
	"io"
	"log"
	"spyderbat-event-forwarder/api"
	"spyderbat-event-forwarder/webhook"
)

type logstats struct {
	recordsRetrieved int
	invalidRecords   int
	loggedRecords    int
}

func (l *logstats) reset() {
	l.recordsRetrieved = 0
	l.invalidRecords = 0
	l.loggedRecords = 0
}

type processLogsRequest struct {
	r        io.Reader        // Input: The data to process
	sapi     api.APIer        // Input: The API service to use for augmenting the data
	eventLog *log.Logger      // Input: The logger to use for emitting events
	webhook  *webhook.Webhook // Input: The webhook to use for emitting events
	stats    *logstats        // Input/Return: stats
}

func processLogs(ctx context.Context, req *processLogsRequest) {
	req.stats.reset()
	scanner := bufio.NewScanner(req.r)

	for scanner.Scan() && ctx.Err() == nil {
		req.stats.recordsRetrieved++
		jsonRecord := scanner.Bytes()

		req.stats.loggedRecords++
		r := req.sapi.AugmentRuntimeDetailsJSON(jsonRecord)
		req.eventLog.Print(string(r))
		req.webhook.Send(r)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("error processing records: %s", err)
	}
}
