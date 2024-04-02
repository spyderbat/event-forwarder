// Spyderbat Event Forwarder
// Copyright (C) 2022-2024 Spyderbat, Inc.
// Use according to license terms.

package record

import (
	"errors"
	"time"

	"github.com/valyala/fastjson"
)

type RecordTime float64

var parserPool = fastjson.ParserPool{}

var ErrInvalidID = errors.New("record does not contain an ID")

// Time returns a native go time (in UTC) from a RecordTime
func (r RecordTime) Time() time.Time { return time.Unix(0, int64(float64(1e9)*float64(r))) }

// RecordTimeFromTime returns a RecordTime from a native go type
func RecordTimeFromTime(t time.Time) RecordTime {
	return RecordTime(float64(t.UnixNano()) / float64(1e9))
}

// SummaryFromJSON returns an ID+version and RecordTime from a JSON byte slice
func SummaryFromJSON(data []byte) (string, RecordTime, error) {
	fj := parserPool.Get()
	defer parserPool.Put(fj)

	v, err := fj.ParseBytes(data)
	if err != nil {
		return "", RecordTime(0), err
	}

	id := string(v.GetStringBytes("id"))
	t := RecordTime(v.GetFloat64("time"))

	if id == "" {
		return "", RecordTime(0), ErrInvalidID
	}

	// IDs are not guaranteed to be unique, so we append the version if it exists
	version := v.GetStringBytes("version")
	if version != nil {
		id = id + ":" + string(version)
	}

	return id, t, nil
}
