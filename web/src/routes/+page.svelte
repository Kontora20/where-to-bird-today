<script lang="ts">
	import { onMount } from 'svelte';
	import Header from '$lib/Header.svelte';
	import HotspotCard from '$lib/HotspotCard.svelte';
	import HotspotList from '$lib/HotspotList.svelte';
	import HotspotMap from '$lib/HotspotMap.svelte';
	import InSeason from '$lib/InSeason.svelte';
	import Scrubber from '$lib/Scrubber.svelte';
	import SpeciesPanel from '$lib/SpeciesPanel.svelte';
	import { loadYourBirds, saveYourBirds } from '$lib/yourBirds';
	import { addDays, formatDay, todayIso, type ScrubberWeek } from '$lib/format';
	import {
		getAreaWeeks,
		getRecommendations,
		getSeason,
		getSpeciesDetail,
		type AreaWeeks as AreaWeeksData,
		type Recommendation,
		type SeasonalSpecies,
		type Season,
		type SpeciesDetail,
		type SpeciesMatch
	} from '$lib/api';

	// The API accepts at most this many target species.
	const maxTargetSpecies = 10;

	// A change of range or date often comes as several in a row (arrow keys in
	// the list, typing in the date), so the search waits for a pause.
	const searchDelayMs = 300;

	const ranges = [
		...[30, 45, 60, 90, 120].map((minutes) => ({ value: `min:${minutes}`, label: `Within ${minutes} min` })),
		...[10, 25, 50, 100].map((km) => ({ value: `km:${km}`, label: `Within ${km} km` }))
	];

	let home = $state({ lat: 54.6872, lon: 25.2797, label: 'Vilnius' });
	let range = $state('min:60');
	let date = $state(todayIso());

	let targetSpecies = $state<SpeciesMatch[]>(loadYourBirds(maxTargetSpecies));

	let recommendations = $state<Recommendation[]>([]);
	// The best weeks of the target species over the area searched; null when
	// the search had no target species.
	let areaWeeks = $state<AreaWeeksData | null>(null);
	
	// What the area holds in the week of the date: null until it has been asked.
	let season = $state<Season | null>(null);
	// What the phone's bottom sheet shows: its three tabs.
	const tabs = [
		{ id: 'hotspots', label: 'Hotspots' },
		{ id: 'season', label: 'In season' },
		{ id: 'year', label: 'The year' }
	] as const;
	let tab = $state<(typeof tabs)[number]['id']>('hotspots');
	let searched = $state(false);
	let loading = $state(false);
	let locating = $state(false);
	let error = $state('');
	let selectedHotspotId = $state<number | null>(null);
	const selectedHotspot = $derived(recommendations.find((hotspot) => hotspot.hotspot_id === selectedHotspotId));

	let openSpecies = $state<SpeciesDetail | null>(null);
	let speciesLoading = $state(false);
	let speciesError = $state('');
	// The hotspot the open species is shown at.
	let speciesHotspotName = $state('');

	onMount(refresh);

	let pendingRefresh: ReturnType<typeof setTimeout> | undefined;
	// The search and the season in progress. Starting another of the same kind
	// aborts it, which also stops the server's work for it, and its answer is
	// never used.
	let searchInProgress: AbortController | undefined;
	let seasonInProgress: AbortController | undefined;

	// Asks again about the area and the date: the search, and the season, which
	// does not depend on Your birds.
	function refresh() {
		clearTimeout(pendingRefresh);
		void runSeason();
		return runSearch();
	}

	function refreshSoon() {
		clearTimeout(pendingRefresh);
		pendingRefresh = setTimeout(refresh, searchDelayMs);
	}

	function areaQuery() {
		const [unit, amount] = range.split(':');
		return {
			lat: home.lat,
			lon: home.lon,
			maxMinutes: unit === 'min' ? Number(amount) : undefined,
			radiusKm: unit === 'km' ? Number(amount) : undefined,
			date: date || undefined
		};
	}

	// A failed season is left out silently: the search shows the same error.
	async function runSeason() {
		seasonInProgress?.abort();
		const { signal } = (seasonInProgress = new AbortController());
		try {
			const found = await getSeason(areaQuery(), signal);
			if (!signal.aborted) season = found;
		} catch {
			if (!signal.aborted) season = null;
		}
	}

	async function runSearch() {
		searchInProgress?.abort();
		const { signal } = (searchInProgress = new AbortController());
		error = '';
		loading = true;
		const query = { ...areaQuery(), speciesIds: targetSpecies.map((target) => target.species_id) };
		try {
			const [found, weeks] = await Promise.all([
				getRecommendations(query, signal),
				query.speciesIds.length > 0 ? getAreaWeeks(query, signal) : null
			]);
			if (signal.aborted) return;
			recommendations = found;
			areaWeeks = weeks;
			
			searched = true;
			if (!found.some((hotspot) => hotspot.hotspot_id === selectedHotspotId)) selectedHotspotId = null;
		} catch (err) {
			if (signal.aborted) return;
			error = err instanceof Error ? err.message : 'search failed';
			recommendations = [];
			areaWeeks = null;
			searched = false;
			selectedHotspotId = null;
		} finally {
			if (!signal.aborted) loading = false;
		}
	}

	function moveHome(lat: number, lon: number, label: string) {
		home = { lat, lon, label };
		return refresh();
	}

	function selectHotspot(hotspotId: number) {
		selectedHotspotId = hotspotId;
		document.getElementById(`hotspot-${hotspotId}`)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
	}

	// The hotspot to show a species at when none was chosen: the selected one,
	// else the highest ranked that expects it, else the highest ranked.
	function hotspotFor(speciesId: number): Recommendation | undefined {
		return (
			selectedHotspot ??
			recommendations.find((hotspot) => hotspot.top_species.some((top) => top.species_id === speciesId)) ??
			recommendations[0]
		);
	}

	function openSpeciesAnywhere(speciesId: number) {
		const hotspot = hotspotFor(speciesId);
		if (hotspot) return openSpeciesDetail(hotspot, speciesId);
		error = 'There is no hotspot in range to show that bird at.';
	}

	async function openSpeciesDetail(hotspot: Recommendation, speciesId: number) {
		speciesHotspotName = hotspot.hotspot_name;
		speciesError = '';
		speciesLoading = true;
		openSpecies = null;
		try {
			openSpecies = await getSpeciesDetail(hotspot.hotspot_id, speciesId, date || undefined);
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
				locating = false;
				moveHome(position.coords.latitude, position.coords.longitude, 'you');
			},
			(geoError) => {
				error = geoError.message;
				locating = false;
			}
		);
	}

	function addTargetSpecies(species: SpeciesMatch) {
		if (
			targetSpecies.length >= maxTargetSpecies ||
			targetSpecies.some((target) => target.species_id === species.species_id)
		) {
			return;
		}
		targetSpecies = [...targetSpecies, species];
		saveYourBirds(targetSpecies);
		return runSearch();
	}

	function toggleSeasonalSpecies(species: SeasonalSpecies) {
		if (targetSpecies.some((target) => target.species_id === species.species_id)) {
			return removeTargetSpecies(species.species_id);
		}
		return addTargetSpecies(species);
	}

	function removeTargetSpecies(speciesId: number) {
		targetSpecies = targetSpecies.filter((target) => target.species_id !== speciesId);
		saveYourBirds(targetSpecies);
		return runSearch();
	}

	// The bars of the year scrubber: how likely Your birds are in each week when
	// there are some, otherwise the checklists recorded in the area.
	const scrubberWeeks = $derived.by((): ScrubberWeek[] => {
		if (areaWeeks && areaWeeks.weeks.length > 0) {
			return areaWeeks.weeks.map((week) => ({
				week: week.week,
				start: week.start,
				value: week.smoothed_frequency,
				emphasized: week.best,
				title: `Week ${week.week}: ${(week.smoothed_frequency * 100).toFixed(0)}% of checklists`
			}));
		}
		return (season?.checklists ?? []).map((week) => ({
			week: week.week,
			start: week.start,
			value: week.checklists,
			emphasized: true,
			title: `Week ${week.week}: ${week.checklists} ${week.checklists === 1 ? 'checklist' : 'checklists'}`
		}));
	});
	const selectedWeek = $derived(scrubberWeeks.findLastIndex((week) => week.start <= date));
	const scrubberNote = $derived(
		areaWeeks && areaWeeks.weeks.length > 0
			? areaWeeks.best_weeks.length > 0
				? `Best weeks for your birds: ${areaWeeks.best_weeks.map((range) => `${formatDay(range.start)} – ${formatDay(range.end)}`).join(', ')}.`
				: 'Bars show how likely your birds are in each week.'
			: 'Bars show the checklists recorded in the area each week.'
	);

	function searchWeek(start: string) {
		date = start;
		return refresh();
	}

	function shiftWeek(weeks: number) {
		date = addDays(date, 7 * weeks);
		refreshSoon();
	}

	function goToToday() {
		date = todayIso();
		return refresh();
	}
</script>

<div class="shell">
	<Header
		searchPlaceholder={targetSpecies.length >= maxTargetSpecies ? 'Limit reached' : 'Search a bird…'}
		searchDisabled={targetSpecies.length >= maxTargetSpecies}
		onpickspecies={addTargetSpecies}
		onerror={(message) => (error = message)}
	/>

	<div class="body">
		<aside class="sidebar">
			<section class="area">
				<h2>Around {home.label}</h2>
				<div class="controls">
					<label class="pill">
						<span class="visually-hidden">How far to travel</span>
						<select bind:value={range} onchange={refreshSoon}>
							{#each ranges as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</select>
					</label>
					<label class="pill">
						<span class="visually-hidden">Date</span>
						<input type="date" bind:value={date} onchange={refreshSoon} />
					</label>
				</div>
				<button type="button" class="link" onclick={useMyLocation} disabled={locating}>
					{locating ? 'Locating…' : 'Use my location'}
				</button>
			</section>

			{#if error}
				<p class="error" role="alert">{error}</p>
			{/if}

			<section class="birds">
				<div class="row">
					<h3>Your birds</h3>
					<span class="count">{targetSpecies.length} / {maxTargetSpecies}</span>
				</div>
				{#if targetSpecies.length > 0}
					<ul class="chips">
						{#each targetSpecies as target (target.species_id)}
							<li>
									<button type="button" class="name" onclick={() => openSpeciesAnywhere(target.species_id)}>
										{target.common_name}
									</button>
									<button
										type="button"
										class="remove"
										onclick={() => removeTargetSpecies(target.species_id)}
										aria-label="Remove {target.common_name}">✕</button
									>
								</li>
						{/each}
					</ul>
				{:else}
					<p class="hint">Search a bird in the box above to rank hotspots for it.</p>
				{/if}
			</section>

			<div class="tabs" role="tablist" aria-label="Sections">
				{#each tabs as item (item.id)}
					<button type="button" role="tab" aria-selected={tab === item.id} onclick={() => (tab = item.id)}>
						{item.label}
					</button>
				{/each}
			</div>

			<section class="hotspots" class:phone-hidden={tab !== 'hotspots'}>
				<div class="row">
					<h3>Hotspots in range</h3>
					<span class="hint">{loading ? 'Searching…' : searched ? `${recommendations.length} found` : ''}</span>
				</div>
				{#if searched && recommendations.length === 0}
					<p class="hint padded">
						{targetSpecies.length > 0
							? 'No hotspot in range has these birds for that week.'
							: 'No hotspots found in range.'}
					</p>
				{/if}
				<HotspotList
					hotspots={recommendations}
					selectedId={selectedHotspotId}
					birds={targetSpecies}
					onselect={selectHotspot}
				/>
			</section>

			<section class="season" class:phone-hidden={tab !== 'season'}>
				<div class="row">
					<h3>{date === todayIso() ? 'In season now' : 'In season that week'}</h3>
					<span class="hint">Peaking around you</span>
				</div>
				{#if season && season.species.length > 0}
					<InSeason
						species={season.species}
						week={season.week}
						birds={targetSpecies}
						full={targetSpecies.length >= maxTargetSpecies}
						ontoggle={toggleSeasonalSpecies}
						onopen={openSpeciesAnywhere}
					/>
				{:else if season}
					<p class="hint padded">Nothing stands out around you that week.</p>
				{/if}
			</section>

			<section class="year-tab" class:phone-hidden={tab !== 'year'}>
				{#if scrubberWeeks.length > 0}
					<Scrubber
						weeks={scrubberWeeks}
						selected={selectedWeek}
						note={scrubberNote}
						onpick={searchWeek}
						onshift={shiftWeek}
						ontoday={goToToday}
					/>
				{/if}
			</section>
		</aside>

		<div class="map-area">
			<HotspotMap
				{home}
				hotspots={recommendations}
				selectedId={selectedHotspotId}
				onselect={selectHotspot}
				onpick={(lat, lon) => moveHome(lat, lon, 'your point')}
			/>
			<p class="legend">
				Bubble: checklists in this week of the year<br />
				Click the map to move your start point
			</p>
			{#if scrubberWeeks.length > 0}
				<div class="scrubber-overlay">
					<Scrubber
						weeks={scrubberWeeks}
						selected={selectedWeek}
						note={scrubberNote}
						onpick={searchWeek}
						onshift={shiftWeek}
						ontoday={goToToday}
					/>
				</div>
			{/if}
			{#if selectedHotspot}
				<HotspotCard
					hotspot={selectedHotspot}
					birds={targetSpecies}
					onclose={() => (selectedHotspotId = null)}
					onpickspecies={(speciesId) => openSpeciesDetail(selectedHotspot, speciesId)}
					onsearchhere={() =>
						moveHome(selectedHotspot.latitude, selectedHotspot.longitude, selectedHotspot.hotspot_name)}
				/>
			{/if}
		</div>
	</div>
</div>

{#if speciesLoading || speciesError || openSpecies}
	<SpeciesPanel
		loading={speciesLoading}
		error={speciesError}
		species={openSpecies}
		hotspotName={speciesHotspotName}
		onclose={closeSpeciesDetail}
	/>
{/if}

<style>
	.shell {
		display: flex;
		flex-direction: column;
		height: 100dvh;
	}

	.body {
		display: flex;
		flex: 1;
		min-height: 0;
	}

	.sidebar {
		display: flex;
		flex-direction: column;
		flex-shrink: 0;
		width: 400px;
		padding-bottom: 1.5rem;
		overflow-y: auto;
		border-right: 1px solid var(--color-line);
		background: var(--color-panel);
	}

	.map-area {
		position: relative;
		flex: 1;
	}

	.area {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 16px 20px 12px;
		border-bottom: 1px solid var(--color-line);
	}

	h2 {
		margin: 0;
		font-size: 1.5rem;
		letter-spacing: -0.01em;
	}

	h3 {
		margin: 0;
		color: var(--color-muted);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.05em;
		text-transform: uppercase;
	}

	.controls {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.pill select,
	.pill input {
		height: 36px;
		padding: 0 12px;
		border: 1px solid var(--color-control-border);
		border-radius: 999px;
		background: var(--color-panel);
		color: var(--color-text);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 600;
	}

	.link {
		align-self: flex-start;
		padding: 0;
		border: 0;
		background: none;
		color: var(--color-accent-text);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 600;
		text-decoration: underline;
		cursor: pointer;
	}

	.link:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
	}

	.error {
		margin: 0;
		padding: 10px 20px 0;
		color: var(--color-error);
	}

	.birds {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 12px 20px;
		border-bottom: 1px solid var(--color-line);
	}

	.row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
	}

	.count {
		color: var(--color-muted);
		font: 0.8rem var(--font-mono);
	}

	.hint {
		margin: 0;
		color: var(--color-muted);
		font-size: 0.85rem;
	}

	.hint.padded {
		padding: 0 20px 8px;
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.chips li {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 4px 6px 4px 12px;
		border-radius: 999px;
		background: var(--color-chip);
		font-size: 0.9rem;
		font-weight: 600;
	}

	.chips .name {
		width: auto;
		height: auto;
		padding: 0;
		font-size: inherit;
		font-weight: inherit;
		cursor: pointer;
	}

	.chips button {
		width: 22px;
		height: 22px;
		border: 0;
		border-radius: 999px;
		background: none;
		color: var(--color-text);
		font: inherit;
		line-height: 1;
		cursor: pointer;
	}

	.hotspots {
		padding-top: 14px;
	}

	.hotspots .row,
	.season .row {
		padding: 0 20px 6px;
	}

	.season {
		padding-top: 14px;
	}

	.tabs,
	.year-tab {
		display: none;
	}

	.scrubber-overlay {
		position: absolute;
		bottom: 16px;
		left: 16px;
		width: min(600px, calc(100% - 96px));
		padding: 14px 16px 12px;
		border-radius: 12px;
		background: var(--color-panel);
		box-shadow: var(--shadow-raised);
	}

	.legend {
		position: absolute;
		top: 16px;
		left: 16px;
		margin: 0;
		padding: 6px 10px;
		border-radius: 8px;
		background: var(--color-panel);
		box-shadow: var(--shadow-raised);
		color: var(--color-muted);
		font-size: 0.8rem;
		line-height: 1.5;
	}

	@media (max-width: 639px) {
		.tabs {
			display: flex;
			padding: 0 8px;
			border-bottom: 1px solid var(--color-line);
		}

		.tabs button {
			flex: 1;
			height: 48px;
			border: 0;
			border-bottom: 3px solid transparent;
			background: none;
			color: var(--color-muted);
			font: inherit;
			font-size: 0.95rem;
			font-weight: 600;
			cursor: pointer;
		}

		.tabs button[aria-selected='true'] {
			border-bottom-color: var(--color-accent-strong);
			color: var(--color-text);
		}

		.year-tab {
			display: block;
			padding: 16px;
		}

		.phone-hidden {
			display: none;
		}

		.scrubber-overlay {
			display: none;
		}
		.sidebar {
			position: fixed;
			right: 0;
			bottom: 0;
			left: 0;
			z-index: 10;
			width: auto;
			height: 45dvh;
			border-right: 0;
			border-radius: 20px 20px 0 0;
			box-shadow: var(--shadow-sheet);
		}

		.legend {
			position: fixed;
			top: 68px;
			left: 12px;
			z-index: 5;
		}
	}
</style>
