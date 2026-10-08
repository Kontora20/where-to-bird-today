import type { SpeciesMatch } from '$lib/api';

const storageKey = 'birds';

// The longest name kept: real names are far shorter, and a name goes into the
// page and, for the language switch, into a request.
const maxNameLength = 100;

function isSpeciesMatch(value: unknown): value is SpeciesMatch {
	const bird = value as Partial<SpeciesMatch> | null;
	return (
		typeof bird?.species_id === 'number' &&
		Number.isSafeInteger(bird.species_id) &&
		bird.species_id > 0 &&
		typeof bird.common_name === 'string' &&
		bird.common_name.length <= maxNameLength &&
		typeof bird.scientific_name === 'string' &&
		bird.scientific_name.length <= maxNameLength
	);
}

// The saved list, or an empty one when nothing usable is saved. What is stored
// is checked, because it lives in the browser where anything can edit it.
export function loadYourBirds(limit: number): SpeciesMatch[] {
	try {
		const saved: unknown = JSON.parse(localStorage.getItem(storageKey) ?? '[]');
		if (!Array.isArray(saved)) return [];
		const birds = new Map<number, SpeciesMatch>();
		for (const bird of saved.filter(isSpeciesMatch)) {
			if (!birds.has(bird.species_id)) birds.set(bird.species_id, bird);
		}
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
