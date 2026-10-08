import { t } from '$lib/i18n.svelte';

// Asks the browser where the reader is. Exactly one of the callbacks runs.
export function locate(onfound: (lat: number, lon: number) => void, onfail: (message: string) => void) {
	if (!navigator.geolocation) {
		onfail(t('error.noGeolocation'));
		return;
	}
	navigator.geolocation.getCurrentPosition(
		(position) => onfound(position.coords.latitude, position.coords.longitude),
		(geoError) => onfail(geoError.message)
	);
}
