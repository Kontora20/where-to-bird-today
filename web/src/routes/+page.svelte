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
	let selectedHotspotId = $state<number | null>(null);

	let mapContainer: HTMLDivElement;
	let leaflet: typeof Leaflet;
	let map: Leaflet.Map;
	let markersLayer: Leaflet.LayerGroup;
	let markersByHotspotId = new Map<number, Leaflet.Marker>();
	let searchLocationMarker: Leaflet.CircleMarker | undefined;

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

		map.on('click', (event) => setSearchLocation(event.latlng.lat, event.latlng.lng));

		updateMarkers();
	});

	// Fills the lat/lon fields and drops (or moves) the search-point pin -- a
	// plain circle, not a pin icon, so it's never confused with a hotspot
	// result marker. Shared by the map click handler and "Use my location".
	function setSearchLocation(latitude: number, longitude: number) {
		lat = latitude.toFixed(4);
		lon = longitude.toFixed(4);
		if (searchLocationMarker) {
			searchLocationMarker.setLatLng([latitude, longitude]);
		} else {
			searchLocationMarker = leaflet
				.circleMarker([latitude, longitude], { radius: 8, color: 'red', fillOpacity: 0.8 })
				.addTo(map);
		}
	}

	function updateMarkers() {
		if (!markersLayer) return;
		markersLayer.clearLayers();
		markersByHotspotId.clear();
		for (const hotspot of recommendations) {
			const marker = leaflet
				.marker([hotspot.latitude, hotspot.longitude])
				.bindPopup(hotspot.hotspot_name)
				.on('click', () => selectHotspot(hotspot.hotspot_id))
				.addTo(markersLayer);
			markersByHotspotId.set(hotspot.hotspot_id, marker);
		}
		if (recommendations.length > 0) {
			map.fitBounds(
				leaflet.latLngBounds(recommendations.map((hotspot) => [hotspot.latitude, hotspot.longitude])),
				{ padding: [20, 20] }
			);
		}
	}

	function selectHotspot(hotspotId: number) {
		selectedHotspotId = hotspotId;
		document.getElementById(`hotspot-${hotspotId}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' });

		const marker = markersByHotspotId.get(hotspotId);
		marker?.openPopup();
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
				setSearchLocation(position.coords.latitude, position.coords.longitude);
				map?.setView([position.coords.latitude, position.coords.longitude], 11);
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

<div class="page">
	<h1>Where to Bird Today</h1>

	<form class="search-form" onsubmit={search}>
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
		<button type="submit" class="primary" disabled={loading}>
			{loading ? 'Searching…' : 'Search'}
		</button>
	</form>

	{#if error}
		<p class="error" role="alert">{error}</p>
	{/if}

	<p class="hint">Click the map to set your location.</p>
	<div bind:this={mapContainer} class="map"></div>

	<ul class="results">
		{#each recommendations as hotspot (hotspot.hotspot_id)}
			<li id={`hotspot-${hotspot.hotspot_id}`} class:selected={hotspot.hotspot_id === selectedHotspotId}>
				<button type="button" onclick={() => selectHotspot(hotspot.hotspot_id)}>
					<strong>{hotspot.hotspot_name}</strong> — {hotspot.distance_km.toFixed(1)} km
				</button>
				<ul class="species">
					{#each hotspot.top_species as species (species.species_id)}
						<li>{species.common_name} ({(species.frequency * 100).toFixed(0)}%)</li>
					{/each}
				</ul>
			</li>
		{/each}
	</ul>
</div>

<style>
	.page {
		max-width: 900px;
		margin: 0 auto;
		padding: 1.5rem 1rem 4rem;
	}

	h1 {
		font-size: 1.5rem;
		margin: 0 0 1rem;
	}

	.search-form {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 0.75rem;
		margin-bottom: 1rem;
	}

	.search-form label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.85rem;
		color: var(--color-muted);
	}

	.search-form input {
		padding: 0.4rem 0.6rem;
		border: 1px solid var(--color-border);
		border-radius: 6px;
		background: var(--color-bg);
		color: var(--color-fg);
		font-size: 1rem;
	}

	.search-form button {
		padding: 0.5rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: 6px;
		background: var(--color-bg);
		color: var(--color-fg);
		font-size: 1rem;
		cursor: pointer;
	}

	.search-form button.primary {
		border-color: var(--color-accent);
		background: var(--color-accent);
		color: #fff;
	}

	.search-form button:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.error {
		color: var(--color-error);
	}

	.hint {
		margin: 0 0 0.5rem;
		font-size: 0.85rem;
		color: var(--color-muted);
	}

	.map {
		height: 400px;
		width: 100%;
		border: 1px solid var(--color-border);
		border-radius: 8px;
		margin-bottom: 1.5rem;
	}

	.results {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.results > li {
		border: 1px solid var(--color-border);
		border-radius: 8px;
		padding: 0.75rem 1rem;
	}

	.results > li.selected {
		background: var(--color-accent-bg);
		outline: 2px solid var(--color-accent);
	}

	.results > li > button {
		all: unset;
		display: block;
		cursor: pointer;
		font-size: 1rem;
	}

	.species {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		list-style: none;
		margin: 0.5rem 0 0;
		padding: 0;
	}

	.species li {
		border-radius: 999px;
		padding: 0.15rem 0.6rem;
		background: var(--color-accent-bg);
		font-size: 0.8rem;
	}
</style>
