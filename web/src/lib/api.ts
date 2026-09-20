// Field names match the Go API's JSON exactly (see
// internal/birding/recommend.go), so there is no mapping layer to keep in
// sync -- just the wire shape, typed.

export interface ExpectedSpecies {
	species_id: number;
	common_name: string;
	scientific_name: string;
	frequency: number;
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

export interface WeekFrequency {
	week: number;
	frequency: number;
}

export interface SpeciesDetail {
	species_id: number;
	common_name: string;
	scientific_name: string;
	birds_of_the_world_url?: string;
	birdlife_datazone_url?: string;
	ebird_url?: string;
	photo?: Photo;
	seasonality: WeekFrequency[];
}

export async function getSpeciesDetail(hotspotId: number, speciesId: number): Promise<SpeciesDetail> {
	const response = await fetch(`/hotspots/${hotspotId}/species/${speciesId}`);
	if (!response.ok) {
		throw new Error(`species detail request failed: ${response.status}`);
	}
	return response.json();
}
