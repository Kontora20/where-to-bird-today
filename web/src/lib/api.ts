import { hotspotName, locale, t } from '$lib/i18n.svelte';
import type { MessageKey } from '$lib/messages';

// Field names match the Go API's JSON exactly (see
// internal/birding/recommend.go), so there is no mapping layer to keep in
// sync -- just the wire shape, typed.

export interface ExpectedSpecies {
	species_id: number;
	common_name: string;
	scientific_name: string;
	frequency: number;
	// How many complete checklists the frequency is out of.
	checklists: number;
}

// The most recent record of one species at a hotspot, with a link to the
// checklist behind it.
export interface LastSeen {
	species_id: number;
	// YYYY-MM-DD
	observed_on: string;
	checklist_url: string;
}

export interface Recommendation {
	hotspot_id: number;
	hotspot_name: string;
	latitude: number;
	longitude: number;
	distance_km: number;
	// Present only when the search was limited by drive time.
	drive_minutes?: number;
	score: number;
	top_species: ExpectedSpecies[];
	// For the species of the search that were ever recorded here, latest first;
	// absent when the search had no species.
	last_seen?: LastSeen[];
	// Absent when the hotspot's source has no public page to link to.
	verification_url?: string;
}

// One of the 52 weeks of the year over an area: the complete checklists there.
export interface WeekTotal {
	week: number;
	// The Monday of the week, YYYY-MM-DD.
	start: string;
	checklists: number;
}

// A species that is much more likely now than it usually is in the area.
export interface SeasonalSpecies {
	species_id: number;
	common_name: string;
	scientific_name: string;
	// The share of the area's checklists this week that recorded it.
	frequency: number;
	peak_week: number;
	// The Monday of its best week, YYYY-MM-DD.
	peak_start: string;
	// From the requested week to its best: 0 is now, above 0 still to come.
	weeks_to_peak: number;
	// The same share for each of the 52 weeks.
	year: number[];
	// Absent when no photo is stored for the species.
	photo?: Photo;
}

export interface Season {
	week: number;
	// Empty when the area has no checklists.
	checklists: WeekTotal[];
	species: SeasonalSpecies[];
}

export interface SpeciesMatch {
	species_id: number;
	common_name: string;
	scientific_name: string;
}

// An error whose message is worded for the reader, in their language.
export class ApiError extends Error {}

// What the server's plain-text error codes say; any other 4xx text is shown
// as it is.
const errorMessages: Record<string, MessageKey> = {
	travel_time_unavailable: 'error.travelTimeUnavailable',
	no_road_near_origin: 'error.noRoad',
	routing_busy: 'error.routingBusy',
	rate_limited: 'error.rateLimited'
};

async function getJson<T>(path: string, params: URLSearchParams, signal?: AbortSignal): Promise<T> {
	params.set('lang', locale.language);
	const response = await fetch(`${path}?${params}`, { signal });
	if (!response.ok) {
		const code = (await response.text()).trim();
		const key = errorMessages[code];
		throw new ApiError(key ? t(key) : response.status < 500 && code ? code : t('error.server'));
	}
	return response.json();
}

export function searchSpecies(query: string): Promise<SpeciesMatch[]> {
	return getJson('/species', new URLSearchParams({ q: query }));
}

// An area and a day, and optionally the species, as every search of an area asks.
export interface SearchQuery {
	lat: number;
	lon: number;
	// Exactly one of these limits the search; maxMinutes wins if both are given.
	maxMinutes?: number;
	radiusKm?: number;
	date?: string;
	speciesIds?: number[];
}

async function getSearch<T>(path: string, query: SearchQuery, signal?: AbortSignal): Promise<T> {
	const params = new URLSearchParams({
		lat: String(query.lat),
		lon: String(query.lon)
	});
	if (query.maxMinutes) {
		params.set('max_minutes', String(query.maxMinutes));
	} else if (query.radiusKm) {
		params.set('radius_km', String(query.radiusKm));
	}
	if (query.date) {
		params.set('date', query.date);
	}
	for (const speciesId of query.speciesIds ?? []) {
		params.append('species', String(speciesId));
	}

	return getJson(path, params, signal);
}

export async function getRecommendations(query: SearchQuery, signal?: AbortSignal): Promise<Recommendation[]> {
	const found = await getSearch<Recommendation[]>('/recommendations', query, signal);
	return found.map((hotspot) => ({ ...hotspot, hotspot_name: hotspotName(hotspot.hotspot_name) }));
}

// The year over the hotspots of an area for the species in the query, which
// must name at least one. For several species each week's frequency is the
// sum of theirs: the number of them a checklist is expected to have.
export interface AreaWeeks {
	// The week of the query's date.
	week: number;
	// Empty when the area has no checklists.
	weeks: WeekFrequency[];
	best_weeks: WeekRange[];
}

export function getAreaWeeks(query: SearchQuery, signal?: AbortSignal): Promise<AreaWeeks> {
	return getSearch('/recommendations/weeks', query, signal);
}

export function getSeason(query: SearchQuery, signal?: AbortSignal): Promise<Season> {
	return getSearch('/in-season', query, signal);
}

export interface Photo {
	image_url: string;
	thumb_url?: string;
	license: string;
	license_url?: string;
	attribution: string;
	attribution_url?: string;
}

// One of the 52 weeks of the year at a hotspot. Week 53 of an ISO year is
// counted in week 52.
export interface WeekFrequency {
	week: number;
	// The Monday of the week, YYYY-MM-DD, in the ISO year of the query's date.
	start: string;
	// The share of the week's checklists that recorded the species; 0 when
	// there were none.
	frequency: number;
	// How many complete checklists the frequency is out of.
	checklists: number;
	// The week pooled with the weeks either side and pulled toward the
	// country's frequency, more strongly the fewer checklists there are. The
	// best weeks are picked by this.
	smoothed_frequency: number;
	best: boolean;
}

// A run of neighbouring best weeks. start and end are YYYY-MM-DD; a run that
// goes past the end of the year ends in the next one.
export interface WeekRange {
	from_week: number;
	to_week: number;
	start: string;
	end: string;
}

export interface HourFrequency {
	// 0 to 23, counted from sunrise: 0 is the first hour after it and 23 the
	// hour before the next one, so the night is the hours in between.
	hours_after_sunrise: number;
	// The hour's clock times on the requested date, as HH:MM. An hour that
	// passes midnight has an end earlier than its start.
	start: string;
	end: string;
	frequency: number;
	// How many complete checklists started in this hour at the hotspot.
	checklists: number;
	// Frequency pulled toward the region's, more strongly the fewer
	// checklists it rests on. The best hours are picked by this.
	adjusted_frequency: number;
}

export interface SpeciesDetail {
	species_id: number;
	common_name: string;
	scientific_name: string;
	birds_of_the_world_url?: string;
	birdlife_datazone_url?: string;
	ebird_url?: string;
	photo?: Photo;
	// Empty when the hotspot has no checklists.
	seasonality: WeekFrequency[];
	best_weeks: WeekRange[];
	time_of_day: HourFrequency[];
	// Absent when the species was never recorded at the hotspot.
	latest_checklist?: { url: string; observed_on: string };
}

// date (YYYY-MM-DD) sets the day the time of day is shown at; today when omitted.
export function getSpeciesDetail(hotspotId: number, speciesId: number, date?: string): Promise<SpeciesDetail> {
	const params = new URLSearchParams();
	if (date) {
		params.set('date', date);
	}
	return getJson(`/hotspots/${hotspotId}/species/${speciesId}`, params);
}
