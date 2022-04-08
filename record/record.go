package record

import "time"

type RecordTime float64

// Time returns a native go time (in UTC) from a RecordTime
func (r RecordTime) Time() time.Time { return time.Unix(0, int64(float64(1e9)*float64(r))) }

// Record represents an event record. The only key we parse out is time.
type Record struct {
	Time RecordTime `json:"time"`
}
