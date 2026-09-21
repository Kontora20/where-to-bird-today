import type { SpeciesMatch } from '$lib/api';

const storageKey = 'birds';

function isSpeciesMatch(value: unknown): value is SpeciesMatch {
	const bird = value as Partial<SpeciesMatch> | null;
	return (
		typeof bird?.species_id === 'number' &&
		typeof bird.common_name === 'string' &&
		typeof bird.scientific_name === 'string'
	);
}

// The saved list, or an empty one when nothing usable is saved. What is stored
// is checked, because it lives in the browser where anything can edit it.
export function loadYourBirds(limit: number): SpeciesMatch[] {
	try {
		const saved: unknown = JSON.parse(localStorage.getItem(storageKey) ?? '[]');
		if (!Array.isArray(saved)) return [];
		const birds = new Map(saved.filter(isSpeciesMatch).map((bird) => [bird.species_id, bird]));
		return [...birds.values()].slice(0, limit);
	} catch {
		return [];
	}
}

export function saveYourBirds(birds: SpeciesMatch[]) {
	try {
		localStorage.setItem(storageKey, JSON.stringify(birds));
	} catch {
		// storage blocked: the list lasts until the tab closes
	}
}
