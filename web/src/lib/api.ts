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
}

export interface RecommendationQuery {
	lat: number;
	lon: number;
	radiusKm: number;
	date?: string;
}

export async function getRecommendations(query: RecommendationQuery): Promise<Recommendation[]> {
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
