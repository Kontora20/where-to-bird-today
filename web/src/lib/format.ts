import type { Recommendation } from '$lib/api';

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
