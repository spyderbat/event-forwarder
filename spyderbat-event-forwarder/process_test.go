// Spyderbat Event Forwarder
// Copyright (C) 2022-2025 Spyderbat, Inc.
// Use according to license terms.

package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"spyderbat-event-forwarder/api"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockSAPI struct {
}

func (a *mockSAPI) AugmentRuntimeDetailsJSON(record []byte) []byte {
	details := &api.RuntimeDetails{CloudInstanceID: "kittens"}

	d, err := json.Marshal(details)
	if err != nil {
		panic(err)
	}

	// This is not pretty, but it avoids the cost of parsing the log record.
	// We can't use append() anywhere, because we cannot modify the original record
	// without corrupting the underlying scanner.
	const key = `,"runtime_details":`
	newlen := len(record) + len(key) + len(d)
	newRecord := make([]byte, newlen)
	copy(newRecord, record[:len(record)-1])
	copy(newRecord[len(record)-1:], key)
	copy(newRecord[len(record)-1+len(key):], d)
	newRecord[len(newRecord)-1] = '}'
	return newRecord
}

func (a *mockSAPI) RefreshSources(ctx context.Context) error {
	return nil
}

func setupLogging(t testing.TB) *bytes.Buffer {
	// redirect log output
	logWriter := log.Writer()
	t.Cleanup(func() {
		log.SetOutput(logWriter)
	})
	logBuf := new(bytes.Buffer)
	log.SetOutput(logBuf)
	return logBuf
}

type nopSeekerCloser struct {
	io.ReadSeeker
}

func (nopSeekerCloser) Close() error { return nil }

func setupTestRequest(t testing.TB) (*processLogsRequest, *bytes.Buffer) {
	eventLogBuf := new(bytes.Buffer)

	req := new(processLogsRequest)
	req.stats = new(logstats)
	req.eventLog = log.New(eventLogBuf, "", 0)
	req.sapi = new(mockSAPI)

	data, err := os.ReadFile("testdata/source_data_response.out")
	require.NoError(t, err)

	req.r = &nopSeekerCloser{bytes.NewReader(data)}

	return req, eventLogBuf
}

func TestProcessLogs(t *testing.T) {
	setupLogging(t)
	req, eventLogBuf := setupTestRequest(t)

	processLogs(context.TODO(), req)

	// all records should be valid
	require.Equal(t, 0, req.stats.invalidRecords)

	// we should have logged all records
	eventLogLines := bytes.Count(eventLogBuf.Bytes(), []byte("\n"))
	require.Equal(t, req.stats.recordsRetrieved, eventLogLines)
	require.Equal(t, req.stats.recordsRetrieved, req.stats.loggedRecords)
}

func BenchmarkProcessLogs(b *testing.B) {
	setupLogging(b)
	req, _ := setupTestRequest(b)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = req.r.(io.Seeker).Seek(0, io.SeekStart)
		processLogs(context.TODO(), req)
	}
}
