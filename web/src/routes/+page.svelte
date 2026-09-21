<script lang="ts">
	import { onMount } from 'svelte';
	import 'leaflet/dist/leaflet.css';
	import type * as Leaflet from 'leaflet';
	import {
		getAreaWeeks,
		getRecommendations,
		getSpeciesDetail,
		searchSpecies,
		type AreaWeeks,
		type HourFrequency,
		type Recommendation,
		type SpeciesDetail,
		type SpeciesMatch
	} from '$lib/api';

	// The API accepts at most this many target species.
	const maxTargetSpecies = 10;

	let lat = $state('');
	let lon = $state('');
	let maxMinutes = $state('60');
	let radiusKm = $state('');
	let date = $state('');

	let targetSpecies = $state<SpeciesMatch[]>([]);
	let speciesQuery = $state('');
	let speciesSuggestions = $state<SpeciesMatch[]>([]);
	let suggestionTimer: ReturnType<typeof setTimeout> | undefined;

	let recommendations = $state<Recommendation[]>([]);
	// The best weeks of the target species over the area searched, and how many
	// species that was; null when the search had no target species.
	let areaWeeks = $state<AreaWeeks | null>(null);
	let areaSpeciesCount = $state(0);
	let searched = $state(false);
	let loading = $state(false);
	let locating = $state(false);
	let error = $state('');
	let selectedHotspotId = $state<number | null>(null);

	let openSpecies = $state<SpeciesDetail | null>(null);
	let speciesLoading = $state(false);
	let speciesError = $state('');

	const hoursAfterSunrise = Array.from({ length: 24 }, (_, index) => index);

	function formatDay(isoDate: string): string {
		return new Date(`${isoDate}T00:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
	}

	// The hours within a tenth of the best one. Neighbouring hours join into one
	// clock range.
	function bestHours(hours: HourFrequency[], highest: number): HourFrequency[] {
		return hours.filter((hour) => highest > 0 && hour.adjusted_frequency >= 0.9 * highest);
	}

	function clockRanges(hours: HourFrequency[]): string[] {
		const ranges: string[] = [];
		let first: HourFrequency | undefined;
		let last: HourFrequency | undefined;
		for (const hour of hours) {
			if (first && last && hour.hours_after_sunrise === last.hours_after_sunrise + 1) {
				last = hour;
				continue;
			}
			if (first && last) ranges.push(`${first.start}–${last.end}`);
			first = last = hour;
		}
		if (first && last) ranges.push(`${first.start}–${last.end}`);
		return ranges;
	}

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

	async function openSpeciesDetail(hotspotId: number, speciesId: number) {
		speciesError = '';
		speciesLoading = true;
		openSpecies = null;
		try {
			openSpecies = await getSpeciesDetail(hotspotId, speciesId, date || undefined);
		} catch (err) {
			speciesError = err instanceof Error ? err.message : 'failed to load species detail';
		} finally {
			speciesLoading = false;
		}
	}

	function closeSpeciesDetail() {
		openSpecies = null;
		speciesError = '';
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

	const speciesLabel = (species: SpeciesMatch) => `${species.common_name} (${species.scientific_name})`;

	// Suggestions are refreshed after a pause in typing. They are a plain list
	// rather than a <datalist> because a browser hides any datalist option that
	// does not contain the typed text, and the server also matches through the
	// other language and through unaccented text.
	function onSpeciesInput() {
		clearTimeout(suggestionTimer);
		const typed = speciesQuery.trim();
		if (typed.length < 2) {
			speciesSuggestions = [];
			return;
		}
		suggestionTimer = setTimeout(async () => {
			try {
				const matches = await searchSpecies(typed);
				// A slower earlier request must not overwrite a newer one.
				if (typed === speciesQuery.trim()) speciesSuggestions = matches;
			} catch (err) {
				error = err instanceof Error ? err.message : 'species search failed';
			}
		}, 200);
	}

	function onSpeciesKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			speciesSuggestions = [];
		} else if (event.key === 'Enter' && speciesSuggestions.length > 0) {
			event.preventDefault();
			addTargetSpecies(speciesSuggestions[0]);
		}
	}

	function addTargetSpecies(species: SpeciesMatch) {
		if (
			targetSpecies.length < maxTargetSpecies &&
			!targetSpecies.some((target) => target.species_id === species.species_id)
		) {
			targetSpecies = [...targetSpecies, species];
		}
		speciesQuery = '';
		speciesSuggestions = [];
	}

	function removeTargetSpecies(speciesId: number) {
		targetSpecies = targetSpecies.filter((target) => target.species_id !== speciesId);
	}

	function search(event: SubmitEvent) {
		event.preventDefault();
		return runSearch();
	}

	// Searches the week a bar of the best weeks chart stands for.
	function searchWeek(start: string) {
		date = start;
		return runSearch();
	}

	async function runSearch() {
		error = '';
		if (!maxMinutes && !radiusKm) {
			error = 'Enter a drive time or a radius';
			return;
		}
		loading = true;
		const query = {
			lat: Number(lat),
			lon: Number(lon),
			maxMinutes: maxMinutes ? Number(maxMinutes) : undefined,
			radiusKm: radiusKm ? Number(radiusKm) : undefined,
			date: date || undefined,
			speciesIds: targetSpecies.map((target) => target.species_id)
		};
		try {
			[recommendations, areaWeeks] = await Promise.all([
				getRecommendations(query),
				query.speciesIds.length > 0 ? getAreaWeeks(query) : null
			]);
			areaSpeciesCount = query.speciesIds.length;
			searched = true;
			updateMarkers();
		} catch (err) {
			error = err instanceof Error ? err.message : 'search failed';
			recommendations = [];
			areaWeeks = null;
			searched = false;
			updateMarkers();
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
			Max drive time (min)
			<input type="number" min="1" max="240" step="any" bind:value={maxMinutes} />
		</label>
		<label>
			Or radius (km)
			<input type="number" min="0" step="any" bind:value={radiusKm} />
		</label>
		<label>
			Date
			<input type="date" bind:value={date} />
		</label>
		<div class="species-field">
			<label>
				Target species
				<input
					type="search"
					autocomplete="off"
					placeholder={targetSpecies.length >= maxTargetSpecies ? 'Limit reached' : 'Type a bird name'}
					disabled={targetSpecies.length >= maxTargetSpecies}
					bind:value={speciesQuery}
					oninput={onSpeciesInput}
					onkeydown={onSpeciesKeydown}
				/>
			</label>
			{#if speciesSuggestions.length > 0}
				<ul class="suggestions">
					{#each speciesSuggestions as suggestion (suggestion.species_id)}
						<li>
							<button type="button" onclick={() => addTargetSpecies(suggestion)}>
								{speciesLabel(suggestion)}
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
		<button type="submit" class="primary" disabled={loading}>
			{loading ? 'Searching…' : 'Search'}
		</button>
	</form>

	{#if targetSpecies.length > 0}
		<ul class="targets" aria-label="Target species">
			{#each targetSpecies as target (target.species_id)}
				<li>
					{target.common_name}
					<button type="button" onclick={() => removeTargetSpecies(target.species_id)} aria-label="Remove {target.common_name}">
						✕
					</button>
				</li>
			{/each}
		</ul>
	{/if}

	{#if error}
		<p class="error" role="alert">{error}</p>
	{/if}

	<p class="hint">Click the map to set your location.</p>
	<div bind:this={mapContainer} class="map"></div>

	{#if areaWeeks && areaWeeks.weeks.length > 0}
		{@const highest = Math.max(...areaWeeks.weeks.map((week) => week.smoothed_frequency))}
		<section class="area-weeks">
			<h2>Best weeks for {areaSpeciesCount === 1 ? 'this species' : 'these species'} in this area</h2>
			<div class="seasonality">
				{#each areaWeeks.weeks as week (week.week)}
					{@const expected =
						areaSpeciesCount === 1
							? `${(week.smoothed_frequency * 100).toFixed(0)}% of checklists`
							: `${week.smoothed_frequency.toFixed(2)} of these species per checklist`}
					<button
						type="button"
						class="week-bar"
						class:best={week.best}
						class:current={week.week === areaWeeks.week}
						style="height: {highest > 0 ? (week.smoothed_frequency / highest) * 100 : 0}%"
						title="Week {week.week}, from {formatDay(week.start)}: {expected} ({week.checklists} {week.checklists === 1
							? 'checklist'
							: 'checklists'} that week)"
						aria-label="Search week {week.week}, from {formatDay(week.start)}"
						onclick={() => searchWeek(week.start)}
					></button>
				{/each}
			</div>
			<p class="chart-note">
				{#if areaWeeks.best_weeks.length > 0}Best: {areaWeeks.best_weeks
						.map((range) => `${formatDay(range.start)} – ${formatDay(range.end)}`)
						.join(', ')}.{/if}
				Each week is pooled with the weeks either side across the hotspots in range. The outlined week is the one searched;
				click a week to search it.
			</p>
		</section>
	{/if}

	{#if searched && recommendations.length === 0}
		<p class="hint">
			{targetSpecies.length > 0 ? 'No hotspot in range has these species for that week.' : 'No hotspots found in range.'}
		</p>
	{/if}

	<ul class="results">
		{#each recommendations as hotspot (hotspot.hotspot_id)}
			<li id={`hotspot-${hotspot.hotspot_id}`} class:selected={hotspot.hotspot_id === selectedHotspotId}>
				<button type="button" onclick={() => selectHotspot(hotspot.hotspot_id)}>
					<strong>{hotspot.hotspot_name}</strong> —
					{#if hotspot.drive_minutes}
						{hotspot.drive_minutes} min drive · {hotspot.distance_km.toFixed(1)} km away
					{:else}
						{hotspot.distance_km.toFixed(1)} km
					{/if}
					{#if targetSpecies.length > 0}
						— {hotspot.score.toFixed(1)} of {targetSpecies.length} targets expected
					{/if}
				</button>
				{#if hotspot.verification_url}
					<a class="verify" href={hotspot.verification_url} target="_blank" rel="noopener noreferrer">
						View on eBird
					</a>
				{/if}
				<ul class="species">
					{#each hotspot.top_species as species (species.species_id)}
						<li>
							<button type="button" onclick={() => openSpeciesDetail(hotspot.hotspot_id, species.species_id)}>
								{species.common_name} ({(species.frequency * 100).toFixed(0)}% of {species.checklists})
							</button>
						</li>
					{/each}
				</ul>
			</li>
		{/each}
	</ul>
</div>

<svelte:window
	onkeydown={(event) => {
		if (event.key === 'Escape' && (speciesLoading || speciesError || openSpecies)) closeSpeciesDetail();
	}}
/>

{#if speciesLoading || speciesError || openSpecies}
	<div
		class="species-panel-backdrop"
		onclick={(event) => event.target === event.currentTarget && closeSpeciesDetail()}
		role="presentation"
	>
		<aside class="species-panel">
			<button type="button" class="close" onclick={closeSpeciesDetail} aria-label="Close">✕</button>

			{#if speciesLoading}
				<p>Loading…</p>
			{:else if speciesError}
				<p class="error" role="alert">{speciesError}</p>
			{:else if openSpecies}
				<h2>{openSpecies.common_name}</h2>
				<p class="scientific-name">{openSpecies.scientific_name}</p>

				{#if openSpecies.photo}
					<figure>
						<img src={openSpecies.photo.image_url} alt={openSpecies.common_name} />
						<figcaption>
							{#if openSpecies.photo.attribution_url}
								<a href={openSpecies.photo.attribution_url} target="_blank" rel="noopener noreferrer">
									{openSpecies.photo.attribution}
								</a>
							{:else}
								{openSpecies.photo.attribution}
							{/if}
							· {openSpecies.photo.license}
						</figcaption>
					</figure>
				{/if}

				{#if openSpecies.seasonality.length > 0}
					{@const highest = Math.max(...openSpecies.seasonality.map((week) => week.smoothed_frequency))}
					<h3>Seasonality at this hotspot</h3>
					<div class="seasonality">
						{#each openSpecies.seasonality as week (week.week)}
							<div
								class="week-bar"
								class:best={week.best}
								style="height: {highest > 0 ? (week.smoothed_frequency / highest) * 100 : 0}%"
								title="Week {week.week}: {(week.smoothed_frequency * 100).toFixed(0)}% expected ({week.checklists === 0
									? 'no checklists here'
									: `${(week.frequency * 100).toFixed(0)}% of ${week.checklists} ${week.checklists === 1 ? 'checklist' : 'checklists'} here`})"
							></div>
						{/each}
					</div>
					<p class="chart-note">
						{#if openSpecies.best_weeks.length > 0}Best: {openSpecies.best_weeks
								.map((range) => `${formatDay(range.start)} – ${formatDay(range.end)}`)
								.join(', ')}.{/if}
						Each week is pooled with the weeks either side; bar height is relative to the best week.
					</p>
				{/if}

				{#if openSpecies.time_of_day.length > 0}
					{@const highest = Math.max(...openSpecies.time_of_day.map((hour) => hour.adjusted_frequency))}
					{@const best = bestHours(openSpecies.time_of_day, highest)}
					<h3>Time of day at this hotspot</h3>
					<div class="time-of-day">
						{#each hoursAfterSunrise as hourAfterSunrise (hourAfterSunrise)}
							{@const match = openSpecies.time_of_day.find((hour) => hour.hours_after_sunrise === hourAfterSunrise)}
							<div
								class="hour-bar"
								class:best={match && best.includes(match)}
								style="height: {match && highest > 0 ? (match.adjusted_frequency / highest) * 100 : 0}%"
								title={match
									? `${match.start}–${match.end}: ${(match.adjusted_frequency * 100).toFixed(0)}% expected (${(match.frequency * 100).toFixed(0)}% of ${match.checklists} ${match.checklists === 1 ? 'checklist' : 'checklists'} here)`
									: 'no data'}
							></div>
						{/each}
					</div>
					<p class="chart-note">
						{#if best.length > 0}Best: {clockRanges(best).join(', ')}.{/if}
						The bars run from sunrise around the clock; their height is relative to the best hour.
					</p>
				{/if}

				<ul class="links">
					{#each [
						{
							url: openSpecies.latest_checklist?.url,
							label: `Latest record at this hotspot (${openSpecies.latest_checklist?.observed_on})`
						},
						{ url: openSpecies.ebird_url, label: 'eBird' },
						{ url: openSpecies.birds_of_the_world_url, label: 'Birds of the World' },
						{ url: openSpecies.birdlife_datazone_url, label: 'BirdLife DataZone' }
					] as link (link.label)}
						{#if link.url}
							<li><a href={link.url} target="_blank" rel="noopener noreferrer">{link.label}</a></li>
						{/if}
					{/each}
				</ul>
			{/if}
		</aside>
	</div>
{/if}

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

	.species-field {
		position: relative;
	}

	.suggestions {
		position: absolute;
		top: 100%;
		left: 0;
		/* Above Leaflet's controls, which go up to z-index 1000. */
		z-index: 1100;
		min-width: 100%;
		width: max-content;
		max-width: min(420px, 90vw);
		margin: 0.2rem 0 0;
		padding: 0.25rem 0;
		list-style: none;
		border: 1px solid var(--color-border);
		border-radius: 6px;
		background: var(--color-bg);
	}

	.suggestions button {
		all: unset;
		display: block;
		box-sizing: border-box;
		width: 100%;
		padding: 0.35rem 0.75rem;
		cursor: pointer;
	}

	.suggestions button:hover,
	.suggestions button:focus-visible {
		background: var(--color-accent-bg);
	}

	.targets {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		list-style: none;
		margin: 0 0 1rem;
		padding: 0;
	}

	.targets li {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.15rem 0.3rem 0.15rem 0.7rem;
		border-radius: 999px;
		background: var(--color-accent-bg);
		font-size: 0.85rem;
	}

	.targets button {
		border: none;
		background: none;
		color: var(--color-fg);
		cursor: pointer;
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
		background: var(--color-accent-bg);
		font-size: 0.8rem;
	}

	.species li button {
		all: unset;
		display: block;
		padding: 0.15rem 0.6rem;
		cursor: pointer;
	}

	.verify {
		display: inline-block;
		margin-top: 0.4rem;
		font-size: 0.8rem;
		color: var(--color-accent);
	}

	.species-panel-backdrop {
		position: fixed;
		inset: 0;
		display: flex;
		justify-content: flex-end;
		background: rgba(0, 0, 0, 0.4);
		/* Above Leaflet's own panes, which go up to z-index 700. */
		z-index: 1000;
	}

	.species-panel {
		position: relative;
		width: min(420px, 100%);
		height: 100%;
		overflow-y: auto;
		padding: 1.5rem;
		background: var(--color-bg);
		border-left: 1px solid var(--color-border);
	}

	.species-panel .close {
		position: absolute;
		top: 1rem;
		right: 1rem;
		border: none;
		background: none;
		font-size: 1.1rem;
		cursor: pointer;
		color: var(--color-fg);
	}

	.species-panel h2 {
		margin: 0 0 0.15rem;
	}

	.scientific-name {
		margin: 0 0 1rem;
		font-style: italic;
		color: var(--color-muted);
	}

	.species-panel figure {
		margin: 0 0 1rem;
	}

	.species-panel img {
		width: 100%;
		border-radius: 8px;
	}

	.species-panel figcaption {
		margin-top: 0.3rem;
		font-size: 0.75rem;
		color: var(--color-muted);
	}

	.species-panel h3 {
		font-size: 0.95rem;
		margin: 1rem 0 0.5rem;
	}

	.seasonality {
		display: flex;
		align-items: flex-end;
		gap: 1px;
		height: 80px;
		border-bottom: 1px solid var(--color-border);
	}

	.time-of-day {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 80px;
		border-bottom: 1px solid var(--color-border);
	}

	.hour-bar {
		flex: 1;
		min-height: 2px;
		background: var(--color-accent-bg);
		border-radius: 2px 2px 0 0;
	}

	.hour-bar.best {
		background: var(--color-accent);
	}

	.chart-note {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin: 0.25rem 0 0;
	}

	.week-bar {
		flex: 1;
		min-height: 2px;
		background: var(--color-accent-bg);
		border-radius: 2px 2px 0 0;
	}

	.week-bar.best {
		background: var(--color-accent);
	}

	button.week-bar {
		border: 0;
		padding: 0;
		cursor: pointer;
	}

	.week-bar.current {
		outline: 2px solid var(--color-fg);
		outline-offset: 1px;
	}

	.area-weeks h2 {
		font-size: 1.1rem;
		margin: 1rem 0 0.5rem;
	}

	.links {
		list-style: none;
		margin: 1rem 0 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.links a {
		color: var(--color-accent);
	}
</style>
