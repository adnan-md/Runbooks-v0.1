package scheduler

import (
	"testing"
	"time"
)

func TestParseField(t *testing.T) {
	m, err := parseField("*", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	if !m[0] || !m[59] {
		t.Error("expected 0 and 59 to be set")
	}

	m, err = parseField("*/15", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	if !m[0] || !m[15] || !m[30] || !m[45] || m[5] {
		t.Error("expected 0,15,30,45 to be set")
	}

	m, err = parseField("5-10", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	if !m[5] || !m[10] || m[11] {
		t.Error("expected 5..10 to be set")
	}
}

func TestNextRun(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := nextRun("0 2 * * *", from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v want %v", next, want)
	}

	next, err = nextRun("*/30 * * * *", from)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v want %v", next, want)
	}
}

func TestNextRunInvalid(t *testing.T) {
	_, err := nextRun("* * *", time.Now())
	if err == nil {
		t.Error("expected error for malformed cron")
	}
}
