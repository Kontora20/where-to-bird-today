package daylight

import (
	"testing"
	"time"
)

// The wanted instants come from MET Norway's sunrise API, an independent
// implementation that rounds to the minute. The sunrise equation is good to
// about a minute, so two minutes is the tolerance.
func TestSunrise(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		latitude  float64
		longitude float64
		date      string
		want      string
	}{
		{"Vilnius midsummer", 54.6872, 25.2797, "2026-06-21", "2026-06-21T01:41:00Z"},
		{"Vilnius midwinter", 54.6872, 25.2797, "2026-12-21", "2026-12-21T06:39:00Z"},
		{"Vilnius equinox", 54.6872, 25.2797, "2026-03-20", "2026-03-20T04:21:00Z"},
		{"Tallinn midsummer", 59.437, 24.7536, "2026-06-21", "2026-06-21T01:03:00Z"},
		{"Tallinn midwinter", 59.437, 24.7536, "2026-12-21", "2026-12-21T07:17:00Z"},
		{"Riga autumn equinox", 56.9496, 24.1052, "2026-09-22", "2026-09-22T04:08:00Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			day, err := time.Parse("2006-01-02", test.date)
			if err != nil {
				t.Fatalf("parsing date: %v", err)
			}
			want, err := time.Parse(time.RFC3339, test.want)
			if err != nil {
				t.Fatalf("parsing want: %v", err)
			}

			got := Sunrise(day, test.latitude, test.longitude)
			if diff := got.Sub(want).Abs(); diff > 2*time.Minute {
				t.Errorf("Sunrise(%s) = %s, want %s (off by %s)", test.date, got.UTC().Format(time.RFC3339), test.want, diff)
			}
		})
	}
}

func TestMinutesAfterSunrise(t *testing.T) {
	t.Parallel()

	zone, err := Zone()
	if err != nil {
		t.Fatalf("Zone(): %v", err)
	}
	const latitude, longitude = 54.6872, 25.2797
	// Vilnius sunrise on this day is 04:41 local time.
	at := func(hour, minute int) time.Time {
		return time.Date(2026, 6, 21, hour, minute, 0, 0, zone)
	}

	tests := []struct {
		name  string
		start time.Time
		want  int
	}{
		{"an hour after sunrise", at(5, 41), 60},
		{"noon", at(12, 0), 439},
		{"just after midnight wraps to the night", at(0, 10), 1169},
		{"just before midnight", at(23, 50), 1149},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := MinutesAfterSunrise(test.start, latitude, longitude)
			if diff := got - test.want; diff < -2 || diff > 2 {
				t.Errorf("MinutesAfterSunrise(%s) = %d, want %d", test.start.Format("15:04"), got, test.want)
			}
		})
	}
}

// A start just before sunrise belongs to the last minutes of the wrapped day,
// however short the time before it: 30 seconds before is minute 1439, and 90
// seconds before is minute 1438.
func TestMinutesAfterSunriseCountsBackFromTheEndOfTheDay(t *testing.T) {
	t.Parallel()

	zone, err := Zone()
	if err != nil {
		t.Fatalf("Zone(): %v", err)
	}
	const latitude, longitude = 54.6872, 25.2797
	sunrise := Sunrise(time.Date(2026, 6, 21, 0, 0, 0, 0, zone), latitude, longitude)

	tests := []struct {
		name   string
		offset time.Duration
		want   int
	}{
		{"30 seconds after sunrise", 30 * time.Second, 0},
		{"30 seconds before sunrise", -30 * time.Second, 1439},
		{"90 seconds before sunrise", -90 * time.Second, 1438},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := MinutesAfterSunrise(sunrise.Add(test.offset), latitude, longitude); got != test.want {
				t.Errorf("MinutesAfterSunrise(sunrise%+v) = %d, want %d", test.offset, got, test.want)
			}
		})
	}
}
