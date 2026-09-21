// Package daylight answers when the sun rises at a place and what the clock
// there says, for the time-of-day profile of a species (D5).
package daylight

import (
	"math"
	"time"

	// The zone database is embedded so LoadLocation works in a container
	// image that ships none.
	_ "time/tzdata"
)

// Zone is the clock all three Baltic countries keep: Vilnius, Riga and
// Tallinn have the same offset and change their clocks on the same days.
func Zone() (*time.Location, error) {
	return time.LoadLocation("Europe/Vilnius")
}

// Sunrise returns when the sun rises on the calendar date of day at the given
// place, in day's time zone. It uses the sunrise equation
// (https://en.wikipedia.org/wiki/Sunrise_equation), accurate to about a
// minute. It is meant for latitudes below the polar circle; further north the
// sun can stay above or below the horizon all day, and the result is then only
// the nearest the equation can give.
func Sunrise(day time.Time, latitude, longitude float64) time.Time {
	const (
		// Sunrise is when the top edge of the sun meets the horizon; the
		// sun's centre is then 0.833 degrees below it (its radius plus the
		// bending of light by the air).
		sunriseAltitude = -0.833
		earthTilt       = 23.4397
	)
	radians := func(degrees float64) float64 { return degrees * math.Pi / 180 }

	// Times below are in days since noon UTC on 2000-01-01.
	j2000 := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	year, month, date := day.Date()
	noonUTC := time.Date(year, month, date, 12, 0, 0, 0, time.UTC)

	solarNoon := noonUTC.Sub(j2000).Hours()/24 - longitude/360
	anomaly := math.Mod(357.5291+0.98560028*solarNoon, 360)
	center := 1.9148*math.Sin(radians(anomaly)) + 0.02*math.Sin(radians(2*anomaly)) + 0.0003*math.Sin(radians(3*anomaly))
	eclipticLongitude := math.Mod(anomaly+center+180+102.9372, 360)
	transit := solarNoon + 0.0053*math.Sin(radians(anomaly)) - 0.0069*math.Sin(radians(2*eclipticLongitude))

	sinDeclination := math.Sin(radians(eclipticLongitude)) * math.Sin(radians(earthTilt))
	cosDeclination := math.Cos(math.Asin(sinDeclination))
	cosHourAngle := (math.Sin(radians(sunriseAltitude)) - math.Sin(radians(latitude))*sinDeclination) /
		(math.Cos(radians(latitude)) * cosDeclination)
	cosHourAngle = max(-1, min(1, cosHourAngle))

	// The hour angle is how far round the day, in radians, the sun is from
	// its highest point when it rises.
	rise := transit - math.Acos(cosHourAngle)/(2*math.Pi)
	return j2000.Add(time.Duration(rise * float64(24*time.Hour))).In(day.Location())
}

// MinutesAfterSunrise is how long after that day's sunrise the clock reads
// start, in whole minutes from 0 to 1439. It wraps at 24 hours, so a start
// just after midnight lands next to one just before it: the night is one
// stretch, not two ends of a line. start's zone must be the place's zone.
func MinutesAfterSunrise(start time.Time, latitude, longitude float64) int {
	// start and its sunrise are on the same calendar date, so the difference
	// is within a day either way and only a start before sunrise needs
	// wrapping. It is wrapped before the conversion because int truncates
	// toward zero: a start 30 seconds before sunrise would otherwise count as
	// minute 0 after it, not minute 1439.
	sinceSunrise := start.Sub(Sunrise(start, latitude, longitude))
	if sinceSunrise < 0 {
		sinceSunrise += 24 * time.Hour
	}
	return int(sinceSunrise.Minutes())
}
