package apitime

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestTime_MarshalJSON_IsEpochSeconds(t *testing.T) {
	src := time.Date(2026, 8, 26, 12, 30, 0, 0, time.UTC)
	tm := New(src)
	b, err := json.Marshal(tm)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := strconv.FormatInt(src.Unix(), 10)
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestTime_RoundTrip(t *testing.T) {
	original := New(time.Date(2026, 8, 26, 12, 30, 0, 0, time.UTC))
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded Time
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !decoded.Time().Equal(original.Time()) {
		t.Errorf("round trip mismatch: got %v, want %v", decoded.Time(), original.Time())
	}
}

func TestTime_ScanFromTimeTime(t *testing.T) {
	var tm Time
	src := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := tm.Scan(src); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !tm.Time().Equal(src) {
		t.Errorf("Scan mismatch: got %v, want %v", tm.Time(), src)
	}
}

func TestTime_ScanNil(t *testing.T) {
	var tm Time
	if err := tm.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
}

func TestTime_ScanRejectsUnsupportedType(t *testing.T) {
	var tm Time
	if err := tm.Scan("not a time"); err == nil {
		t.Error("expected an error scanning a string, got nil")
	}
}
