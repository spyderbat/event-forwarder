// Spyderbat Event Forwarder
// Copyright (C) 2022-2023 Spyderbat, Inc.
// Use according to license terms.

package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"spyderbat-event-forwarder/api"
	"spyderbat-event-forwarder/config"
	"testing"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/stretchr/testify/assert"
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

	cfg := new(config.Config)
	cfg.OrgUID = "test"
	cfg.APIKey = "test"

	req := new(processLogsRequest)
	req.cfg = cfg
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

	err := processLogs(req)
	require.NoError(t, err)

	// all records should be valid
	require.Equal(t, 0, req.stats.invalidRecords)

	// we should have logged all records
	eventLogLines := bytes.Count(eventLogBuf.Bytes(), []byte("\n"))
	require.Equal(t, req.stats.newRecords, eventLogLines)
	require.Equal(t, req.stats.newRecords, req.stats.loggedRecords)
	require.Equal(t, 0, req.stats.filteredRecords)

	// processing the same file again should result in no new records (deduplication)
	req, _ = setupTestRequest(t)
	start := time.Now()
	err = processLogs(req)
	dur := time.Since(start)
	require.NoError(t, err)
	require.Equal(t, 0, req.stats.newRecords)
	t.Logf("processed %d records in %s (%.2f records/sec)", req.stats.recordsRetrieved, dur, float64(req.stats.recordsRetrieved)/dur.Seconds())
}

type exprTest struct {
	expr          string
	expectedStats logstats
	expectedError bool
}

var exprTests = []exprTest{
	{
		"true",
		logstats{
			recordsRetrieved: 5574,
			newRecords:       5574,
			invalidRecords:   0,
			filteredRecords:  0,
			loggedRecords:    5574,
		},
		false,
	},
	{
		"false", logstats{
			recordsRetrieved: 5574,
			newRecords:       5574,
			invalidRecords:   0,
			filteredRecords:  5574,
			loggedRecords:    0,
		},
		false,
	},
	{
		"version + 1",
		logstats{},
		true,
	},
	{
		"badfield > 0",
		logstats{
			recordsRetrieved: 5574,
			newRecords:       5574,
			invalidRecords:   0,
			filteredRecords:  0,
			loggedRecords:    5574,
		},
		false,
	},
	{
		`(
			 schema startsWith "model_spydertrace:"
			 and
			 (score ?? 0) > 1000
		 )
		 or
		 (
			 not
			 (
			     schema startsWith "model_spydertrace:"
			     or
			     schema startsWith "event_redflag:bogons:"
			     or
			     (severity ?? "") in ["info", "low", "medium"]
			 )
		)`,
		logstats{
			recordsRetrieved: 5574,
			newRecords:       5574,
			invalidRecords:   0,
			filteredRecords:  5387,
			loggedRecords:    187,
		},
		false,
	},
}

func TestProcessLogsWithExpr(t *testing.T) {
	outBuf := setupLogging(t)

	for n, test := range exprTests {
		outBuf.Reset()
		t.Logf("testcase %d", n)

		// reset the LRU
		lruCache = lru.New(dedupCacheElements)

		req, eventLogBuf := setupTestRequest(t)
		req.cfg.Expr = test.expr
		err := req.cfg.PrepareAndValidate()
		if test.expectedError {
			require.Error(t, err)
			t.Logf("got the expected error, next testcase")
			t.Logf("%s", err.Error())
			continue
		}
		require.NoError(t, err)

		t.Logf("testing expr: %s", req.cfg.GetExprProgram().Source.Content())
		t.Logf("\n%s", outBuf.String())

		start := time.Now()
		err = processLogs(req)
		dur := time.Since(start)
		require.NoError(t, err)
		t.Logf("processed %d records in %s (%.2f records/sec)", req.stats.recordsRetrieved, dur, float64(req.stats.recordsRetrieved)/dur.Seconds())

		// all records should be valid
		require.Equal(t, 0, req.stats.invalidRecords)

		// we should have logged all records
		eventLogLines := bytes.Count(eventLogBuf.Bytes(), []byte("\n"))
		require.Equal(t, req.stats.loggedRecords, eventLogLines)

		// verify stats
		req.stats.last = 0
		require.Equal(t, test.expectedStats, *req.stats)
	}

}

type regexTest struct {
	regex         []string
	expectedStats logstats
	expectedError bool
}

var regexTests = []regexTest{
	{
		[]string{`^.*$`},
		logstats{
			recordsRetrieved: 5574,
			newRecords:       5574,
			invalidRecords:   0,
			filteredRecords:  0,
			loggedRecords:    5574,
		},
		false,
	},
	{
		[]string{`^$`},
		logstats{
			recordsRetrieved: 5574,
			newRecords:       5574,
			invalidRecords:   0,
			filteredRecords:  5574,
			loggedRecords:    0,
		},
		false,
	},
	{
		[]string{`event_audit:email:`, `event_agentflag:bat_status:`},
		logstats{
			recordsRetrieved: 5574,
			newRecords:       5574,
			invalidRecords:   0,
			filteredRecords:  5553,
			loggedRecords:    21,
		},
		false,
	},
	{
		[]string{`(`},
		logstats{},
		true,
	},
}

func TestProcessLogsWithRegex(t *testing.T) {
	outBuf := setupLogging(t)

	for n, test := range regexTests {
		outBuf.Reset()
		t.Logf("testcase %d", n)

		// reset the LRU
		lruCache = lru.New(dedupCacheElements)

		req, eventLogBuf := setupTestRequest(t)
		req.cfg.MatchRegex = test.regex
		err := req.cfg.PrepareAndValidate()
		if test.expectedError {
			require.Error(t, err)
			t.Logf("got the expected error, next testcase")
			t.Logf("%s", err.Error())
			continue
		}
		require.NoError(t, err)

		t.Logf("testing regex: %s", req.cfg.GetRegexes())

		t.Logf("\n%s", outBuf.String())

		assert.Equal(t, len(req.cfg.GetRegexes()), len(test.regex))

		start := time.Now()
		err = processLogs(req)
		dur := time.Since(start)
		assert.NoError(t, err)
		t.Logf("processed %d records in %s (%.2f records/sec)", req.stats.recordsRetrieved, dur, float64(req.stats.recordsRetrieved)/dur.Seconds())

		// all records should be valid
		assert.Equal(t, 0, req.stats.invalidRecords)

		// we should have logged all records
		eventLogLines := bytes.Count(eventLogBuf.Bytes(), []byte("\n"))
		assert.Equal(t, req.stats.loggedRecords, eventLogLines)

		// verify stats
		req.stats.last = 0
		assert.Equal(t, test.expectedStats, *req.stats)
	}

}

func BenchmarkProcessLogs(b *testing.B) {
	var err error
	setupLogging(b)
	req, _ := setupTestRequest(b)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = req.r.(io.Seeker).Seek(0, io.SeekStart)
		lruCache = lru.New(dedupCacheElements)
		err = processLogs(req)
		require.NoError(b, err)
	}
}

func BenchmarkProcessLogsWithExpr(b *testing.B) {
	setupLogging(b)
	req, _ := setupTestRequest(b)
	req.cfg.Expr = `true`
	err := req.cfg.PrepareAndValidate()
	require.NoError(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = req.r.(io.Seeker).Seek(0, io.SeekStart)
		lruCache = lru.New(dedupCacheElements)
		err = processLogs(req)
		require.NoError(b, err)
	}
}
