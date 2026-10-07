<script lang="ts">
	import { onMount, tick } from 'svelte';
	import Header from '$lib/Header.svelte';
	import HotspotCard from '$lib/HotspotCard.svelte';
	import HotspotList from '$lib/HotspotList.svelte';
	import HotspotMap from '$lib/HotspotMap.svelte';
	import InSeason from '$lib/InSeason.svelte';
	import PlanSearch, { type Plan } from '$lib/PlanSearch.svelte';
	import RangePicker from '$lib/RangePicker.svelte';
	import Scrubber from '$lib/Scrubber.svelte';
	import SpeciesPanel from '$lib/SpeciesPanel.svelte';
	import { loadYourBirds, saveYourBirds } from '$lib/yourBirds';
	import {
		addDays,
		formatDay,
		rangeLabel,
		todayIso,
		type MapLayer,
		type ScrubberWeek
	} from '$lib/format';
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
	let mapLayer = $state<MapLayer>('all');
	let planOpen = $state(false);

	// The map's legend shows for a few seconds, on load and after the layer
	// changes, then gets out of the way; the i button brings it back.
	const legendSeconds = 6;
	let legendOpen = $state(true);
	let legendTimer: ReturnType<typeof setTimeout> | undefined;

	function showLegend() {
		legendOpen = true;
		clearTimeout(legendTimer);
		legendTimer = setTimeout(() => (legendOpen = false), legendSeconds * 1000);
	}

	function toggleLegend() {
		if (!legendOpen) return showLegend();
		clearTimeout(legendTimer);
		legendOpen = false;
	}

	$effect(() => {
		void mapLayer;
		showLegend();
		return () => clearTimeout(legendTimer);
	});
	// On a phone the range and the date are folded into one line to leave room
	// for the lists; on a wider screen they are always open.
	let controlsOpen = $state(false);
	const controlsSummary = $derived.by(() => {
		const [unit, amount] = range.split(':');
		const reach = unit === 'min' ? `${rangeLabel('min', Number(amount))} by car` : `${rangeLabel('km', Number(amount))} straight line`;
		return `${reach} · ${date === todayIso() ? 'Today' : formatDay(date)}`;
	});

	// "Your birds" has nothing to size bubbles by once the list is empty, so
	// the switch falls back rather than showing every bubble at minimum size.
	$effect(() => {
		if (targetSpecies.length === 0) mapLayer = 'all';
	});
	let seasonSection: HTMLElement;
	const seasonTitle = $derived(date === todayIso() ? 'In season now' : 'In season that week');
	const seasonCount = $derived(season?.species.length ?? 0);
	// The first few names of the In season list, for the line under the controls.
	const seasonTeaser = $derived.by(() => {
		const names = (season?.species ?? []).slice(0, 3).map((species) => species.common_name).join(', ');
		return seasonCount > 3 ? `${names} and ${seasonCount - 3} more` : names;
	});

	// On a phone the section is a tab, so it is opened before it is scrolled to.
	async function showSeason() {
		tab = 'season';
		await tick();
		seasonSection.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	const selectedHotspot = $derived(recommendations.find((hotspot) => hotspot.hotspot_id === selectedHotspotId));

	let openSpecies = $state<SpeciesDetail | null>(null);
	let speciesLoading = $state(false);
	let speciesError = $state('');
	// The hotspot the open species is shown at.
	let speciesHotspotName = $state('');

	onMount(refresh);

	// The tabs are only on screen on a phone, so the sections are only tab
	// panels there; elsewhere they are plain sections.
	let phone = $state(false);
	onMount(() => {
		const query = matchMedia('(max-width: 639px)');
		const update = () => (phone = query.matches);
		update();
		query.addEventListener('change', update);
		return () => query.removeEventListener('change', update);
	});

	// Arrow keys move between the tabs, Home and End jump to the first and last.
	function moveTab(event: KeyboardEvent) {
		const last = tabs.length - 1;
		const index = tabs.findIndex((item) => item.id === tab);
		const next =
			event.key === 'ArrowRight' ? (index === last ? 0 : index + 1)
			: event.key === 'ArrowLeft' ? (index === 0 ? last : index - 1)
			: event.key === 'Home' ? 0
			: event.key === 'End' ? last
			: -1;
		if (next < 0) return;
		event.preventDefault();
		tab = tabs[next].id;
		document.getElementById(`tab-${tab}`)?.focus();
	}

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

	// Applies everything the Plan a search panel set at once, as one search.
	function applyPlan(plan: Plan) {
		home = plan.home;
		range = plan.range;
		date = plan.date;
		targetSpecies = plan.birds;
		saveYourBirds(targetSpecies);
		planOpen = false;
		return refresh();
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

<svelte:head>
	<title>Where to Bird Today</title>
</svelte:head>

<div class="shell" inert={planOpen || speciesLoading || !!speciesError || !!openSpecies}>
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
				<button type="button" class="summary" aria-expanded={controlsOpen} onclick={() => (controlsOpen = !controlsOpen)}>
					{controlsSummary}
					<span aria-hidden="true">{controlsOpen ? '▴' : '▾'}</span>
				</button>
				<div class="fold" class:open={controlsOpen}>
					<RangePicker
						value={range}
						place={home.label}
						onchange={(next) => {
							range = next;
							refreshSoon();
						}}
					/>
					<div class="controls">
						<label class="pill">
							<span class="visually-hidden">Date</span>
							<input type="date" bind:value={date} onchange={refreshSoon} />
						</label>
					</div>
				</div>
				<div class="links">
					<button type="button" class="link" onclick={useMyLocation} disabled={locating}>
						{locating ? 'Locating…' : 'Use my location'}
					</button>
					<button type="button" class="link" onclick={() => (planOpen = true)}>Plan a search</button>
					{#if seasonCount > 0}
						<button type="button" class="link season-link" onclick={showSeason}>In season ↓</button>
					{/if}
				</div>
				{#if seasonCount > 0}
					<button type="button" class="teaser" onclick={showSeason}>
						<strong>{seasonTitle}:</strong>
						{seasonTeaser}
					</button>
				{/if}
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

			<div class="tabs" role="tablist" aria-label="Sections" tabindex="-1" onkeydown={moveTab}>
				{#each tabs as item (item.id)}
					<button
						type="button"
						role="tab"
						id="tab-{item.id}"
						aria-controls="panel-{item.id}"
						aria-selected={tab === item.id}
						tabindex={tab === item.id ? 0 : -1}
						onclick={() => (tab = item.id)}
					>
						{item.id === 'season' && seasonCount > 0 ? `${item.label} (${seasonCount})` : item.label}
					</button>
				{/each}
			</div>

			<section
				class="hotspots"
				class:phone-hidden={tab !== 'hotspots'}
				id="panel-hotspots"
				role={phone ? 'tabpanel' : undefined}
				aria-labelledby={phone ? 'tab-hotspots' : undefined}
			>
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

			<section
				class="season"
				class:phone-hidden={tab !== 'season'}
				id="panel-season"
				role={phone ? 'tabpanel' : undefined}
				aria-labelledby={phone ? 'tab-season' : undefined}
				bind:this={seasonSection}
			>
				<div class="row">
					<h3>{seasonTitle}</h3>
					<span class="hint">Peaking around you</span>
				</div>
				{#if targetSpecies.length === 0 && seasonCount > 0}
					<p class="hint padded">Tap + to add a bird to Your birds. The bars show its chance through the year.</p>
				{/if}
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

			<section
				class="year-tab"
				class:phone-hidden={tab !== 'year'}
				id="panel-year"
				role={phone ? 'tabpanel' : undefined}
				aria-labelledby={phone ? 'tab-year' : undefined}
			>
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
				birds={targetSpecies}
				layer={mapLayer}
				onselect={selectHotspot}
				onpick={(lat, lon) => moveHome(lat, lon, 'your point')}
			/>
			<div class="layer-switch" role="group" aria-label="Map layer">
				<button type="button" aria-pressed={mapLayer === 'all'} onclick={() => (mapLayer = 'all')}>
					All hotspots
				</button>
				<button
					type="button"
					aria-pressed={mapLayer === 'yourBirds'}
					disabled={targetSpecies.length === 0}
					onclick={() => (mapLayer = 'yourBirds')}
				>
					Your birds
				</button>
			</div>
			<button
				type="button"
				class="legend-button"
				aria-label="What the map shows"
				aria-expanded={legendOpen}
				aria-controls="map-legend"
				onclick={toggleLegend}>i</button
			>
			{#if legendOpen}
				<p id="map-legend" class="legend">
					Bubble: {mapLayer === 'yourBirds' ? 'how many of your birds were seen here' : 'birdwatcher lists in this week of the year'}<br
					/>
					Click the map to move your start point
				</p>
			{/if}
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

{#if planOpen}
	<PlanSearch
		{home}
		{range}
		{date}
		birds={targetSpecies}
		maxBirds={maxTargetSpecies}
		onapply={applyPlan}
		onclose={() => (planOpen = false)}
	/>
{/if}

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

	.summary {
		display: none;
	}

	.fold {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.links {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 16px;
	}

	.teaser {
		padding: 8px 12px;
		border: 0;
		border-radius: 10px;
		background: var(--color-chip);
		color: var(--color-text);
		font: inherit;
		font-size: 0.85rem;
		line-height: 1.4;
		text-align: left;
		cursor: pointer;
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

	.legend-button {
		position: absolute;
		top: 16px;
		left: 16px;
		z-index: 5;
		width: 36px;
		height: 36px;
		border: 0;
		border-radius: 999px;
		background: var(--color-panel);
		box-shadow: var(--shadow-raised);
		color: var(--color-text);
		font: italic 700 1rem serif;
		cursor: pointer;
	}

	.legend {
		position: absolute;
		top: 60px;
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

	.layer-switch {
		position: absolute;
		top: 16px;
		right: 16px;
		display: flex;
		gap: 2px;
		padding: 2px;
		border-radius: 8px;
		background: var(--color-control-track);
		box-shadow: var(--shadow-raised);
	}

	.layer-switch button {
		padding: 6px 10px;
		border: none;
		border-radius: 6px;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}

	.layer-switch button[aria-pressed='true'] {
		background: var(--color-inverse);
		color: var(--color-on-inverse);
	}

	.layer-switch button:disabled {
		color: var(--color-muted);
		cursor: not-allowed;
	}

	@media (max-width: 639px) {
		.tabs {
			position: sticky;
			top: 0;
			z-index: 3;
			display: flex;
			padding: 0 8px;
			border-bottom: 1px solid var(--color-line);
			background: var(--color-panel);
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

		.summary {
			display: flex;
			align-items: center;
			justify-content: space-between;
			min-height: 44px;
			padding: 0 14px;
			border: 1px solid var(--color-control-border);
			border-radius: 10px;
			background: var(--color-panel);
			color: var(--color-text);
			font: inherit;
			font-weight: 600;
			cursor: pointer;
		}

		.fold:not(.open) {
			display: none;
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
			top: 120px;
			left: 12px;
			z-index: 5;
		}

		.legend-button {
			position: fixed;
			top: 68px;
			left: 12px;
			width: 44px;
			height: 44px;
		}

		.link {
			min-height: 44px;
		}

		.season-link {
			display: none;
		}

		.layer-switch button {
			min-height: 44px;
		}

		.layer-switch {
			position: fixed;
			top: 68px;
			right: 12px;
			z-index: 5;
		}
	}
</style>
