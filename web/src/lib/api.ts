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
	score: number;
	top_species: ExpectedSpecies[];
	// Absent when the hotspot's source has no public page to link to.
	verification_url?: string;
}

export async function getRecommendations(query: {
	lat: number;
	lon: number;
	radiusKm: number;
	date?: string;
}): Promise<Recommendation[]> {
	const params = new URLSearchParams({
		lat: String(query.lat),
		lon: String(query.lon),
		radius_km: String(query.radiusKm)
	});
	if (query.date) {
		params.set('date', query.date);
	}

	const response = await fetch(`/recommendations?${params}`);
	if (!response.ok) {
		throw new Error(`recommendations request failed: ${response.status}`);
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
