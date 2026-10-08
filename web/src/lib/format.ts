import { formatDate, formatNumber, t } from '$lib/i18n.svelte';
import type { MessageKey } from '$lib/messages';
import type { LastSeen, Recommendation, SeasonalSpecies, SpeciesMatch } from '$lib/api';

// The address when it is an https link, otherwise nothing. Links come from
// the server, which got some of them from outside; a "javascript:" address
// would run when followed.
export function httpsUrl(value: string | undefined): string | undefined {
	try {
		return value && new URL(value).protocol === 'https:' ? value : undefined;
	} catch {
		return undefined;
	}
}

// How many checklists are too few to trust a share.
export const fewChecklists = 5;

// The words for the share of a week's checklists that recorded a bird. Words
// say less than a number, which suits shares that rest on few checklists.
export function chanceKey(frequency: number): MessageKey {
	if (frequency >= 0.8) return 'chance.almostAlways';
	if (frequency >= 0.5) return 'chance.often';
	if (frequency >= 0.2) return 'chance.sometimes';
	if (frequency >= 0.05) return 'chance.rarely';
	return 'chance.veryRarely';
}

// A bird seen within this many days counts as seen this week.
const recentDays = 7;
// Older than this, a date reads better than a number of days.
const relativeDays = 14;
const millisecondsPerDay = 86_400_000;

export function formatDay(isoDate: string): string {
	return formatDate(new Date(`${isoDate}T00:00:00`));
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
	const km = rangeLabel('km', hotspot.distance_km, 1);
	return hotspot.drive_minutes ? `${rangeLabel('min', hotspot.drive_minutes)} · ${km}` : km;
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
	if (days <= 0) return t('seen.today');
	if (days === 1) return t('seen.yesterday');
	if (days <= relativeDays) return t('seen.daysAgo', { count: days });
	const date = new Date(`${isoDate}T00:00:00`);
	return formatDate(date, { year: date.getFullYear() !== today.getFullYear() });
}

export interface Sighting {
	text: string;
	// Seen within the last week.
	recent: boolean;
	url?: string;
}

// One bird's last record at a hotspot, worded for the hotspot card.
export function sighting(lastSeen: LastSeen | undefined, today: Date): Sighting {
	if (!lastSeen) return { text: t('seen.never'), recent: false };
	const days = daysSince(lastSeen.observed_on, today);
	const when = seenText(lastSeen.observed_on, today);
	const weekday = formatDate(new Date(`${lastSeen.observed_on}T00:00:00`), { weekday: true });
	return {
		text: days <= relativeDays ? `${when} · ${weekday}` : t('seen.last', { when }),
		recent: days <= recentDays,
		url: httpsUrl(lastSeen.checklist_url)
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
	if (!latest) return { text: t('seen.noneOfYours'), recent: false };
	const name = birds.find((bird) => bird.species_id === latest.species_id)?.common_name ?? t('seen.aBird');
	const recent = daysSince(latest.observed_on, today) <= recentDays;
	return {
		text: t(recent ? 'seen.recent' : 'seen.old', { name, when: seenText(latest.observed_on, today) }),
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

// The dates the page accepts. The year is limited because Date.UTC reads the
// years 0 to 99 as 1900 to 1999, and a six-digit year is not a date the server
// takes.
export const dateLimits = { min: '2000-01-01', max: '2100-12-31' };

// Whether value is a real day, as YYYY-MM-DD, within dateLimits. An empty date
// input, which the browser allows, is not one.
export function isUsableDate(value: string): boolean {
	if (!/^\d{4}-\d\d-\d\d$/.test(value) || value < dateLimits.min || value > dateLimits.max) return false;
	return addDays(value, 0) === value;
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
// What a typed-in range may be: the server's caps, birding.MaxDriveMinutes and
// birding.MaxRadiusKM.
export const rangeLimits = { min: { low: 5, high: 120 }, km: { low: 1, high: 300 } };

export function rangeLabel(unit: 'min' | 'km', amount: number, fractionDigits = 0): string {
	return t(unit === 'min' ? 'range.min' : 'range.km', { amount: formatNumber(amount, fractionDigits) });
}

// When a species is at its best, worded for the In season list.
export function peakText(species: SeasonalSpecies): string {
	const weeks = species.weeks_to_peak;
	if (Math.abs(weeks) <= 1) return t('season.peakingNow');
	const day = formatDay(species.peak_start);
	return weeks > 0 ? t('season.peaksOn', { day, weeks }) : t('season.peakedOn', { day, weeks: -weeks });
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
