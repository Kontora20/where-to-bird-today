package timeofday

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Kontora20/where-to-bird-today/internal/daylight"
)

// On 2026-03-29 the clocks in Vilnius go forward at 03:00, so a clock reading
// of 04:30 that day is 04:30 on the wall, not 05:30.
func TestLocalStartOnClockChangeDay(t *testing.T) {
	t.Parallel()

	zone, err := daylight.Zone()
	if err != nil {
		t.Fatalf("Zone(): %v", err)
	}
	observedOn := time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC)
	fourThirty := pgtype.Time{Microseconds: (4*time.Hour + 30*time.Minute).Microseconds(), Valid: true}

	got := localStart(observedOn, fourThirty, zone)

	if got.Format("2006-01-02 15:04:05") != "2026-03-29 04:30:00" {
		t.Errorf("localStart() = %s, want 2026-03-29 04:30:00", got.Format("2006-01-02 15:04:05"))
	}
}
