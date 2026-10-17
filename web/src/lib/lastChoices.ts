const storageKey = 'last-choices';

export interface LastChoices {
	home: { lat: number; lon: number; label: string };
	range: string;
	poolWeeks: boolean;
}

type Home = LastChoices['home'];

// Inside Lithuania's bounding box with margin: the search stays inside one country.
function isHome(value: unknown): value is Home {
	const home = value as Partial<Home> | null;
	return (
		typeof home?.lat === 'number' &&
		typeof home.lon === 'number' &&
		home.lat >= 53 &&
		home.lat <= 57 &&
		home.lon >= 20 &&
		home.lon <= 28 &&
		typeof home.label === 'string' &&
		home.label.length <= 100
	);
}

// The saved area and range, or null when nothing usable is saved. What is
// stored is checked, because it lives in the browser where anything can edit it.
export function loadLastChoices(): LastChoices | null {
	try {
		const saved = JSON.parse(localStorage.getItem(storageKey) ?? 'null') as Partial<LastChoices> | null;
		if (!isHome(saved?.home) || typeof saved.range !== 'string' || !/^(min|km):\d{1,3}$/.test(saved.range)) {
			return null;
		}
		const { lat, lon, label } = saved.home;
		return { home: { lat, lon, label }, range: saved.range, poolWeeks: saved.poolWeeks === true };
	} catch {
		return null;
	}
}

export function saveLastChoices(choices: LastChoices) {
	try {
		localStorage.setItem(storageKey, JSON.stringify(choices));
	} catch {
		// storage blocked: the choices last until the tab closes
	}
}
