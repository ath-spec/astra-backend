// Package apitime is the single place every domain converts between
// Postgres/Go time.Time and the wire format this API uses for dates and
// timestamps: Unix epoch seconds (int64), UTC. Every request and response
// field that represents a point in time — order timestamps, booking/maturity
// dates, created_at, deadlines, and so on — uses this convention rather than
// ISO strings, so clients never have to deal with date-format parsing.
package apitime

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"time"
)

// Time is a drop-in replacement for time.Time in every domain struct that
// crosses the wire: it marshals to/from JSON as Unix epoch seconds (not an
// ISO string), while still scanning from and encoding to Postgres
// TIMESTAMPTZ/DATE columns exactly like time.Time does (via Scan/Value), so
// provider code that already does `var t time.Time; row.Scan(&t)` only ever
// needs its variable's type changed to apitime.Time — no query changes.
type Time time.Time

func New(t time.Time) Time { return Time(t) }

// NewPtr converts a *time.Time (as scanned from a nullable DB column) to a
// *Time, preserving nil. Provider code should scan nullable
// TIMESTAMPTZ/DATE columns into a local *time.Time and convert with this
// afterward, rather than scanning directly into **Time — pgx's automatic
// Scanner-fallback isn't guaranteed for pointer-to-pointer destinations.
func NewPtr(t *time.Time) *Time {
	if t == nil {
		return nil
	}
	at := New(*t)
	return &at
}

// ToTimePtr converts a *Time back to a *time.Time, for passing as a query
// argument — the mirror of NewPtr, for the same reason (avoiding reliance on
// driver.Valuer support for pointer-to-Valuer argument types).
func ToTimePtr(t *Time) *time.Time {
	if t == nil {
		return nil
	}
	tt := t.Time()
	return &tt
}

func (t Time) Time() time.Time { return time.Time(t) }

func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(time.Time(t).UTC().Unix(), 10)), nil
}

func (t *Time) UnmarshalJSON(data []byte) error {
	var seconds int64
	if err := parseInt64(data, &seconds); err != nil {
		return fmt.Errorf("apitime: decode epoch seconds: %w", err)
	}
	*t = Time(time.Unix(seconds, 0).UTC())
	return nil
}

func parseInt64(data []byte, out *int64) error {
	v, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	*out = v
	return nil
}

// Scan implements sql.Scanner so pgx (which falls back to database/sql
// scanning for types it doesn't natively recognize) can decode a
// TIMESTAMPTZ/DATE column straight into an apitime.Time.
func (t *Time) Scan(value any) error {
	switch v := value.(type) {
	case time.Time:
		*t = Time(v)
		return nil
	case nil:
		*t = Time{}
		return nil
	default:
		return fmt.Errorf("apitime.Time: cannot scan %T", value)
	}
}

// Value implements driver.Valuer so an apitime.Time can be passed directly
// as a query argument the same way a time.Time can.
func (t Time) Value() (driver.Value, error) {
	return time.Time(t), nil
}

// Epoch converts a time.Time to Unix epoch seconds.
func Epoch(t time.Time) int64 { return t.UTC().Unix() }

// EpochPtr converts a *time.Time to *int64, preserving nil.
func EpochPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	e := t.UTC().Unix()
	return &e
}

// FromEpoch converts Unix epoch seconds to a UTC time.Time.
func FromEpoch(seconds int64) time.Time { return time.Unix(seconds, 0).UTC() }

// FromEpochPtr converts a client-supplied *int64 to *time.Time, preserving nil.
func FromEpochPtr(seconds *int64) *time.Time {
	if seconds == nil {
		return nil
	}
	t := FromEpoch(*seconds)
	return &t
}
