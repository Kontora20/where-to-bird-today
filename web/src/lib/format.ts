import type { LastSeen, Recommendation, SpeciesMatch } from '$lib/api';

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
