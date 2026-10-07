import type { LastSeen, Recommendation, SeasonalSpecies, SpeciesMatch } from '$lib/api';

// A bird seen within this many days counts as seen this week.
const recentDays = 7;
// Older than this, a date reads better than a number of days.
const relativeDays = 14;
const millisecondsPerDay = 86_400_000;

export function formatDay(isoDate: string): string {
	return new Date(`${isoDate}T00:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
}

// Every species row of a hotspot rests on the same checklists, so the first
// one tells how many checklists the hotspot has in the searched week.
export function checklistCount(hotspot: Recommendation): number {
	return hotspot.top_species[0]?.checklists ?? 0;
}

// How many of birds have ever been recorded at hotspot -- the map's "Your
// birds" bubble size and badge count.
export function birdCount(hotspot: Recommendation): number {
	return hotspot.last_seen?.length ?? 0;
}

// What a map bubble's size means: every hotspot's checklists this week of the
// year, or just this search's birds.
export type MapLayer = 'all' | 'yourBirds';

export function distanceText(hotspot: Recommendation): string {
	const km = `${hotspot.distance_km.toFixed(1)} km`;
	return hotspot.drive_minutes ? `${hotspot.drive_minutes} min · ${km}` : km;
}

// Whole days from an ISO date to today, counted in the reader's own calendar.
export function daysSince(isoDate: string, today: Date): number {
	const [year, month, day] = isoDate.split('-').map(Number);
	const then = Date.UTC(year, month - 1, day);
	const now = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate());
	return Math.round((now - then) / millisecondsPerDay);
}

// "today", "yesterday", "5 days ago", or the date itself once it is over two
// weeks back, with the year when it is not this year's.
export function seenText(isoDate: string, today: Date): string {
	const days = daysSince(isoDate, today);
	if (days <= 0) return 'today';
	if (days === 1) return 'yesterday';
	if (days <= relativeDays) return `${days} days ago`;
	const date = new Date(`${isoDate}T00:00:00`);
	return date.toLocaleDateString(undefined, {
		day: 'numeric',
		month: 'short',
		year: date.getFullYear() === today.getFullYear() ? undefined : 'numeric'
	});
}

export interface Sighting {
	text: string;
	// Seen within the last week.
	recent: boolean;
	url?: string;
}

// One bird's last record at a hotspot, worded for the hotspot card.
export function sighting(lastSeen: LastSeen | undefined, today: Date): Sighting {
	if (!lastSeen) return { text: 'never recorded here', recent: false };
	const days = daysSince(lastSeen.observed_on, today);
	const when = seenText(lastSeen.observed_on, today);
	const weekday = new Date(`${lastSeen.observed_on}T00:00:00`).toLocaleDateString(undefined, {
		weekday: 'short',
		day: 'numeric',
		month: 'short'
	});
	return {
		text: days <= relativeDays ? `${when} · ${weekday}` : `last seen ${when}`,
		recent: days <= recentDays,
		url: lastSeen.checklist_url
	};
}

// The status line of a hotspot in the list: the most recent record of any of
// the birds, or null when the search has no birds.
export function lastSeenStatus(
	hotspot: Recommendation,
	birds: SpeciesMatch[],
	today: Date
): { text: string; recent: boolean } | null {
	if (birds.length === 0) return null;
	let latest: LastSeen | undefined;
	for (const seen of hotspot.last_seen ?? []) {
		if (!latest || seen.observed_on > latest.observed_on) latest = seen;
	}
	if (!latest) return { text: 'None of your birds recorded here', recent: false };
	const name = birds.find((bird) => bird.species_id === latest.species_id)?.common_name ?? 'A bird of yours';
	const recent = daysSince(latest.observed_on, today) <= recentDays;
	return {
		text: `${name} ${recent ? 'seen' : 'last seen'} ${seenText(latest.observed_on, today)}`,
		recent
	};
}

// The map's badge for a hotspot: how many of birds were ever recorded there,
// and whether any of them was within the last week. Null when there is
// nothing to badge.
export function birdBadge(hotspot: Recommendation, birds: SpeciesMatch[], today: Date): { count: number; recent: boolean } | null {
	if (birds.length === 0 || !hotspot.last_seen?.length) return null;
	const recent = hotspot.last_seen.some((seen) => daysSince(seen.observed_on, today) <= recentDays);
	return { count: hotspot.last_seen.length, recent };
}

// Today as YYYY-MM-DD in the reader's calendar. 'sv' formats a date that way.
export function todayIso(): string {
	return new Date().toLocaleDateString('sv');
}

// The date a number of days after an ISO date, as YYYY-MM-DD.
export function addDays(isoDate: string, days: number): string {
	const [year, month, day] = isoDate.split('-').map(Number);
	return new Date(Date.UTC(year, month - 1, day + days)).toISOString().slice(0, 10);
}

// The first Saturday after an ISO date, as YYYY-MM-DD.
export function nextSaturday(isoDate: string): string {
	const [year, month, day] = isoDate.split('-').map(Number);
	const weekday = new Date(Date.UTC(year, month - 1, day)).getUTCDay();
	return addDays(isoDate, (6 - weekday + 7) % 7 || 7);
}

// The ways to limit a search, as the value "min:60" or "km:25".
export const driveMinutes = [30, 45, 60, 90, 120];
export const radiusKm = [10, 25, 50, 100];
// What a typed-in range may be. The server caps drive time at 120 minutes
// (birding.MaxDriveMinutes) and puts no cap on a radius, so the UI does.
export const rangeLimits = { min: { low: 5, high: 120 }, km: { low: 1, high: 150 } };

export function rangeLabel(unit: 'min' | 'km', amount: number): string {
	return unit === 'min' ? `${amount} min` : `${amount} km`;
}

// When a species is at its best, worded for the In season list.
export function peakText(species: SeasonalSpecies): string {
	const weeks = species.weeks_to_peak;
	if (Math.abs(weeks) <= 1) return 'Peaking now';
	const day = formatDay(species.peak_start);
	return weeks > 0 ? `Peaks ${day}, in ${weeks} weeks` : `Peaked ${day}, ${-weeks} weeks ago`;
}

export interface SparkBar {
	// Pixels.
	height: number;
	kind: 'now' | 'high' | 'low';
}

// The bars of a species' year in fortnights: the higher week of each pair, so
// a short peak is not averaged away. The bar of the fortnight of the week is
// marked, and so are those within 70% of the highest.
export function sparkBars(year: number[], week: number, maxHeight = 28): SparkBar[] {
	const bins = Array.from({ length: Math.floor(year.length / 2) }, (_, i) => Math.max(year[2 * i], year[2 * i + 1]));
	const highest = Math.max(...bins);
	const now = Math.floor((week - 1) / 2);
	return bins.map((value, i) => ({
		height: highest > 0 ? Math.max(3, Math.round((value / highest) * maxHeight)) : 3,
		kind: i === now ? 'now' : value >= 0.7 * highest ? 'high' : 'low'
	}));
}

// One bar of the year scrubber.
export interface ScrubberWeek {
	week: number;
	// The Monday of the week, YYYY-MM-DD.
	start: string;
	value: number;
	// Drawn in the accent colour rather than the plain bar colour.
	emphasized: boolean;
	title: string;
}
