package server

import (
	"testing"
	"time"
)

func TestShouldFire_EveryMinute(t *testing.T) {
	if !shouldFire("* * * * *", time.Date(2026, 3, 18, 14, 30, 0, 0, time.UTC)) {
		t.Error("* * * * * should always fire")
	}
}

func TestShouldFire_SpecificMinute(t *testing.T) {
	if !shouldFire("30 * * * *", time.Date(2026, 3, 18, 14, 30, 0, 0, time.UTC)) {
		t.Error("30 * * * * should fire at :30")
	}
	if shouldFire("30 * * * *", time.Date(2026, 3, 18, 14, 15, 0, 0, time.UTC)) {
		t.Error("30 * * * * should not fire at :15")
	}
}

func TestShouldFire_Every5Minutes(t *testing.T) {
	if !shouldFire("*/5 * * * *", time.Date(2026, 3, 18, 14, 0, 0, 0, time.UTC)) {
		t.Error("*/5 should fire at :00")
	}
	if !shouldFire("*/5 * * * *", time.Date(2026, 3, 18, 14, 15, 0, 0, time.UTC)) {
		t.Error("*/5 should fire at :15")
	}
	if shouldFire("*/5 * * * *", time.Date(2026, 3, 18, 14, 13, 0, 0, time.UTC)) {
		t.Error("*/5 should not fire at :13")
	}
}

func TestShouldFire_HourAndMinute(t *testing.T) {
	if !shouldFire("0 9 * * *", time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)) {
		t.Error("0 9 * * * should fire at 09:00")
	}
	if shouldFire("0 9 * * *", time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)) {
		t.Error("0 9 * * * should not fire at 10:00")
	}
}

func TestShouldFire_Range(t *testing.T) {
	if !shouldFire("0 9-17 * * *", time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)) {
		t.Error("9-17 should match hour 12")
	}
	if shouldFire("0 9-17 * * *", time.Date(2026, 3, 18, 20, 0, 0, 0, time.UTC)) {
		t.Error("9-17 should not match hour 20")
	}
}

func TestShouldFire_CommaList(t *testing.T) {
	if !shouldFire("0,15,30,45 * * * *", time.Date(2026, 3, 18, 14, 15, 0, 0, time.UTC)) {
		t.Error("0,15,30,45 should match :15")
	}
	if shouldFire("0,15,30,45 * * * *", time.Date(2026, 3, 18, 14, 10, 0, 0, time.UTC)) {
		t.Error("0,15,30,45 should not match :10")
	}
}

func TestShouldFire_DayOfWeek(t *testing.T) {
	// 2026-03-18 is a Wednesday (day 3)
	if !shouldFire("0 9 * * 3", time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)) {
		t.Error("day 3 (Wednesday) should match")
	}
	if shouldFire("0 9 * * 1", time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)) {
		t.Error("day 1 (Monday) should not match on Wednesday")
	}
}

func TestShouldFire_InvalidSchedule(t *testing.T) {
	if shouldFire("invalid", time.Now()) {
		t.Error("invalid schedule should not fire")
	}
	if shouldFire("* *", time.Now()) {
		t.Error("incomplete schedule should not fire")
	}
}

func TestFormatCronDescription(t *testing.T) {
	tests := []struct {
		schedule string
		want     string
	}{
		{"* * * * *", "every minute"},
		{"0 * * * *", "every hour"},
		{"*/5 * * * *", "every 5 minutes"},
		{"0 9 * * 1-5", "0 9 * * 1-5"}, // complex — returns as-is
	}
	for _, tt := range tests {
		got := FormatCronDescription(tt.schedule)
		if got != tt.want {
			t.Errorf("FormatCronDescription(%q) = %q, want %q", tt.schedule, got, tt.want)
		}
	}
}
