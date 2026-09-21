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
	// Absent when the hotspot's source has no public page to link to.
	verification_url?: string;
}

export interface SpeciesMatch {
	species_id: number;
	common_name: string;
	scientific_name: string;
}

export async function searchSpecies(query: string): Promise<SpeciesMatch[]> {
	const response = await fetch(`/species?${new URLSearchParams({ q: query })}`);
	if (!response.ok) {
		throw new Error(`species search failed: ${response.status}`);
	}
	return response.json();
}

export async function getRecommendations(query: {
	lat: number;
	lon: number;
	// Exactly one of these limits the search; maxMinutes wins if both are given.
	maxMinutes?: number;
	radiusKm?: number;
	date?: string;
	speciesIds?: number[];
}): Promise<Recommendation[]> {
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

	const response = await fetch(`/recommendations?${params}`);
	if (!response.ok) {
		// The server's plain-text reason ("no road near that location") is
		// meant to be shown.
		throw new Error((await response.text()).trim() || `recommendations request failed: ${response.status}`);
	}
	return response.json();
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
export async function getSpeciesDetail(hotspotId: number, speciesId: number, date?: string): Promise<SpeciesDetail> {
	const params = new URLSearchParams();
	if (date) {
		params.set('date', date);
	}
	const response = await fetch(`/hotspots/${hotspotId}/species/${speciesId}?${params}`);
	if (!response.ok) {
		throw new Error(`species detail request failed: ${response.status}`);
	}
	return response.json();
}
