<script lang="ts">
	import { onMount } from 'svelte';
	import 'leaflet/dist/leaflet.css';
	import type * as Leaflet from 'leaflet';
	import { getRecommendations, type Recommendation } from '$lib/api';

	let lat = $state('');
	let lon = $state('');
	let radiusKm = $state('25');
	let date = $state('');

	let recommendations = $state<Recommendation[]>([]);
	let loading = $state(false);
	let locating = $state(false);
	let error = $state('');

	let mapContainer: HTMLDivElement;
	let leaflet: typeof Leaflet;
	let map: Leaflet.Map;
	let markersLayer: Leaflet.LayerGroup;

	// Leaflet touches `window`, so it's loaded here rather than at module
	// scope -- onMount only runs in the browser, never during the static
	// build (ssr is off for this whole app, but this keeps the import from
	// depending on that).
	onMount(async () => {
		leaflet = await import('leaflet');

		// Vite bundles Leaflet's default marker icons under hashed asset
		// paths, but the plugin's own CSS still points at the plain
		// unbundled filenames -- without this, markers render with a
		// broken-image icon instead of the pin.
		delete (leaflet.Icon.Default.prototype as { _getIconUrl?: unknown })._getIconUrl;
		leaflet.Icon.Default.mergeOptions({
			iconRetinaUrl: (await import('leaflet/dist/images/marker-icon-2x.png')).default,
			iconUrl: (await import('leaflet/dist/images/marker-icon.png')).default,
			shadowUrl: (await import('leaflet/dist/images/marker-shadow.png')).default
		});

		map = leaflet.map(mapContainer).setView([54.6872, 25.2797], 8);
		leaflet
			.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
				attribution:
					'&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
			})
			.addTo(map);
		markersLayer = leaflet.layerGroup().addTo(map);

		updateMarkers();
	});

	function updateMarkers() {
		if (!markersLayer) return;
		markersLayer.clearLayers();
		for (const hotspot of recommendations) {
			leaflet
				.marker([hotspot.latitude, hotspot.longitude])
				.bindPopup(hotspot.hotspot_name)
				.addTo(markersLayer);
		}
		if (recommendations.length > 0) {
			map.fitBounds(
				leaflet.latLngBounds(recommendations.map((hotspot) => [hotspot.latitude, hotspot.longitude])),
				{ padding: [20, 20] }
			);
		}
	}

	function useMyLocation() {
		if (!navigator.geolocation) {
			error = 'geolocation is not supported by this browser';
			return;
		}
		error = '';
		locating = true;
		navigator.geolocation.getCurrentPosition(
			(position) => {
				lat = position.coords.latitude.toFixed(4);
				lon = position.coords.longitude.toFixed(4);
				locating = false;
			},
			(geoError) => {
				error = geoError.message;
				locating = false;
			}
		);
	}

	async function search(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		loading = true;
		try {
			recommendations = await getRecommendations({
				lat: Number(lat),
				lon: Number(lon),
				radiusKm: Number(radiusKm),
				date: date || undefined
			});
			updateMarkers();
		} catch (err) {
			error = err instanceof Error ? err.message : 'search failed';
		} finally {
			loading = false;
		}
	}
</script>

<h1>Where to Bird Today</h1>

<form onsubmit={search}>
	<label>
		Latitude
		<input type="number" step="any" bind:value={lat} required />
	</label>
	<label>
		Longitude
		<input type="number" step="any" bind:value={lon} required />
	</label>
	<button type="button" onclick={useMyLocation} disabled={locating}>
		{locating ? 'Locating…' : 'Use my location'}
	</button>
	<label>
		Radius (km)
		<input type="number" step="any" bind:value={radiusKm} required />
	</label>
	<label>
		Date
		<input type="date" bind:value={date} />
	</label>
	<button type="submit" disabled={loading}>{loading ? 'Searching…' : 'Search'}</button>
</form>

{#if error}
	<p role="alert">{error}</p>
{/if}

<div bind:this={mapContainer} style="height: 400px; width: 100%;"></div>

<ul>
	{#each recommendations as hotspot (hotspot.hotspot_id)}
		<li>
			<strong>{hotspot.hotspot_name}</strong> — {hotspot.distance_km.toFixed(1)} km
			<ul>
				{#each hotspot.top_species as species (species.species_id)}
					<li>{species.common_name} ({(species.frequency * 100).toFixed(0)}%)</li>
				{/each}
			</ul>
		</li>
	{/each}
</ul>
