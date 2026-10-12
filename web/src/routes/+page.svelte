<script lang="ts">
	import { onMount, tick, untrack } from 'svelte';
	import Header from '$lib/Header.svelte';
	import HotspotList from '$lib/HotspotList.svelte';
	import HotspotMap from '$lib/HotspotMap.svelte';
	import InSeason from '$lib/InSeason.svelte';
	import PlanSearch, { type Plan } from '$lib/PlanSearch.svelte';
	import RangePicker from '$lib/RangePicker.svelte';
	import Scrubber from '$lib/Scrubber.svelte';
	import SpeciesPanel from '$lib/SpeciesPanel.svelte';
	import { locale, placeLabel, t } from '$lib/i18n.svelte';
	import { locate } from '$lib/locate';
	import { loadYourBirds, saveYourBirds } from '$lib/yourBirds';
	import {
		addDays,
		dateLimits,
		formatDay,
		isUsableDate,
		rangeLabel,
		todayIso,
		type MapLayer,
		type ScrubberWeek
	} from '$lib/format';
	import {
		ApiError,
		getAreaWeeks,
		getRecommendations,
		getSeason,
		getSpeciesDetail,
		searchSpecies,
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
	const tabs = [{ id: 'hotspots' }, { id: 'season' }, { id: 'year' }] as const;
	let tab = $state<(typeof tabs)[number]['id']>('hotspots');
	let searched = $state(false);
	let loading = $state(false);
	let locating = $state(false);
	let error = $state('');
	let selectedHotspotId = $state<number | null>(null);
	let mapLayer = $state<MapLayer>('all');
	let planOpen = $state(false);

	// On a phone the range and the date are folded into one line to leave room
	// for the lists; on a wider screen they are always open.
	let controlsOpen = $state(false);
	const dateText = $derived(date === todayIso() ? t('common.today') : formatDay(date));
	const controlsSummary = $derived.by(() => {
		const [unit, amount] = range.split(':');
		const reach =
			unit === 'min'
				? t('area.summaryCar', { amount: rangeLabel('min', Number(amount)) })
				: t('area.summaryStraight', { amount: rangeLabel('km', Number(amount)) });
		return `${reach} · ${dateText}`;
	});

	// "Your birds" has nothing to size bubbles by once the list is empty, so
	// the switch falls back rather than showing every bubble at minimum size.
	$effect(() => {
		if (targetSpecies.length === 0) mapLayer = 'all';
	});
	let seasonSection: HTMLElement;
	const seasonTitle = $derived(t(date === todayIso() ? 'season.titleNow' : 'season.titleThen'));
	const seasonCount = $derived(season?.species.length ?? 0);
	// The first few names of the In season list, for the line under the controls.
	const seasonTeaser = $derived.by(() => {
		const names = (season?.species ?? []).slice(0, 3).map((species) => species.common_name).join(', ');
		return seasonCount > 3 ? t('area.teaserMore', { names, count: seasonCount - 3 }) : names;
	});

	// On a phone the section is a tab, so it is opened before it is scrolled to.
	async function showSeason() {
		tab = 'season';
		await tick();
		seasonSection.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	// What the caption under the map says about the area searched.
	const mapCaption = $derived.by(() => {
		const [unit, amount] = range.split(':');
		return t(unit === 'min' ? 'range.captionDrive' : 'range.captionStraight', {
			amount: Number(amount),
			place: placeLabel(home.label)
		});
	});

	let openSpecies = $state<SpeciesDetail | null>(null);
	let speciesLoading = $state(false);
	let speciesError = $state('');
	// The hotspot the open species is shown at.
	let speciesHotspotName = $state('');

	onMount(refresh);

	// Bird names, hotspot names and the season come in the chosen language, so
	// a change of language asks for them again.
	let shownLanguage = locale.language;
	$effect(() => {
		const language = locale.language;
		if (language === shownLanguage) return;
		shownLanguage = language;
		untrack(() => {
			void renameYourBirds();
			void refresh();
		});
	});

	// Your birds is saved with the names of the language it was added in; each
	// bird is looked up again by its scientific name for the new one. The new
	// names are put onto the list as it is when the lookups end, so a bird
	// removed meanwhile stays removed, and a rename that a later language
	// switch has replaced is dropped.
	let latestRename = 0;
	async function renameYourBirds() {
		const thisRename = ++latestRename;
		const renamed = new Map<number, SpeciesMatch>();
		await Promise.all(
			targetSpecies.map(async (bird) => {
				try {
					const matches = await searchSpecies(bird.scientific_name);
					const match = matches.find((candidate) => candidate.species_id === bird.species_id);
					if (match) renamed.set(bird.species_id, match);
				} catch {
					// the bird keeps the name it has
				}
			})
		);
		if (thisRename !== latestRename) return;
		targetSpecies = targetSpecies.map((bird) => renamed.get(bird.species_id) ?? bird);
		saveYourBirds(targetSpecies);
	}

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
			error = err instanceof ApiError ? err.message : t('error.searchFailed');
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

	function selectHotspot(hotspotId: number | null) {
		selectedHotspotId = hotspotId;
	}

	const selectedHotspot = $derived(recommendations.find((hotspot) => hotspot.hotspot_id === selectedHotspotId));

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
		error = t('error.noHotspotForBird');
	}

	// The species request in progress. Closing the panel aborts it, so a slow
	// answer neither keeps the page locked nor opens the panel again later.
	let speciesInProgress: AbortController | undefined;

	async function openSpeciesDetail(hotspot: Recommendation, speciesId: number) {
		speciesInProgress?.abort();
		const { signal } = (speciesInProgress = new AbortController());
		speciesHotspotName = hotspot.hotspot_name;
		speciesError = '';
		speciesLoading = true;
		openSpecies = null;
		try {
			const detail = await getSpeciesDetail(hotspot.hotspot_id, speciesId, date || undefined, signal);
			if (!signal.aborted) openSpecies = detail;
		} catch (err) {
			if (signal.aborted) return;
			speciesError = err instanceof ApiError ? err.message : t('error.speciesDetailFailed');
		} finally {
			if (!signal.aborted) speciesLoading = false;
		}
	}

	function closeSpeciesDetail() {
		speciesInProgress?.abort();
		speciesLoading = false;
		openSpecies = null;
		speciesError = '';
	}

	function useMyLocation() {
		error = '';
		locating = true;
		locate(
			(lat, lon) => {
				locating = false;
				moveHome(lat, lon, 'you');
			},
			(message) => {
				locating = false;
				error = message;
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
				title: t('year.barShare', { week: week.week, percent: (week.smoothed_frequency * 100).toFixed(0) })
			}));
		}
		return (season?.checklists ?? []).map((week) => ({
			week: week.week,
			start: week.start,
			value: week.checklists,
			emphasized: true,
			title: t('year.barCount', { week: week.week, count: week.checklists })
		}));
	});
	const selectedWeek = $derived(scrubberWeeks.findLastIndex((week) => week.start <= date));
	const scrubberNote = $derived(
		areaWeeks && areaWeeks.weeks.length > 0
			? areaWeeks.best_weeks.length > 0
				? t('year.noteBest', {
						ranges: areaWeeks.best_weeks.map((range) => `${formatDay(range.start)} – ${formatDay(range.end)}`).join(', ')
					})
				: t('year.noteBirds')
			: t('year.noteArea')
	);

	function searchWeek(start: string) {
		date = start;
		return refresh();
	}

	function shiftWeek(weeks: number) {
		const shifted = addDays(date, 7 * weeks);
		if (isUsableDate(shifted)) date = shifted;
		refreshSoon();
	}

	// The date box can be emptied or typed into a date the page cannot use.
	function changeDate() {
		if (!isUsableDate(date)) date = todayIso();
		refreshSoon();
	}

	function goToToday() {
		date = todayIso();
		return refresh();
	}
</script>

<svelte:head>
	<title>{t('app.title')}</title>
</svelte:head>

{#snippet yearScrubber()}
	<Scrubber
		weeks={scrubberWeeks}
		selected={selectedWeek}
		note={scrubberNote}
		onpick={searchWeek}
		onshift={shiftWeek}
		ontoday={goToToday}
	/>
{/snippet}

<div class="shell" inert={planOpen || speciesLoading || !!speciesError || !!openSpecies}>
	<Header
		searchPlaceholder={t(targetSpecies.length >= maxTargetSpecies ? 'search.limit' : 'search.placeholder')}
		searchDisabled={targetSpecies.length >= maxTargetSpecies}
		onpickspecies={addTargetSpecies}
		onerror={(message) => (error = message)}
	/>

	<div class="body">
		<aside class="sidebar">
			<section class="area">
				<p class="eyebrow">{t('area.label')} · {dateText}</p>
				<h2 class="place">{placeLabel(home.label)}</h2>
				<button type="button" class="summary" aria-expanded={controlsOpen} onclick={() => (controlsOpen = !controlsOpen)}>
					{controlsSummary}
					<span aria-hidden="true">{controlsOpen ? '▴' : '▾'}</span>
				</button>
				<div class="fold" class:open={controlsOpen}>
					<RangePicker
						value={range}
						onchange={(next) => {
							range = next;
							refreshSoon();
						}}
					/>
					<label class="date">
						<span class="visually-hidden">{t('common.date')}</span>
						<input type="date" min={dateLimits.min} max={dateLimits.max} bind:value={date} onchange={changeDate} />
					</label>
				</div>
				<div class="links">
					<button type="button" class="link" onclick={useMyLocation} disabled={locating}>
						{locating ? t('area.locating') : t('area.useMyLocation')}
					</button>
					<button type="button" class="link" onclick={() => (planOpen = true)}>{t('area.plan')}</button>
					{#if seasonCount > 0}
						<button type="button" class="link season-link" onclick={showSeason}>{t('area.seasonLink')}</button>
					{/if}
				</div>
				{#if seasonCount > 0}
					<button type="button" class="teaser" onclick={showSeason}>
						<strong>{seasonTitle}:</strong>
						{seasonTeaser}
					</button>
				{/if}
			</section>
			<div class="double-rule" aria-hidden="true"></div>

			{#if error}
				<p class="error" role="alert">{error}</p>
			{/if}

			<section class="birds">
				<div class="row">
					<h3 class="section-heading">{t('common.yourBirds')}</h3>
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
									aria-label={t('common.remove', { name: target.common_name })}>×</button
								>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="hint">{t('birds.hint')}</p>
				{/if}
			</section>

			<div class="tabs" role="tablist" aria-label={t('tabs.label')} tabindex="-1" onkeydown={moveTab}>
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
						{item.id === 'season' && seasonCount > 0
							? t('tabs.withCount', { label: t('tabs.season'), count: seasonCount })
							: t(`tabs.${item.id}`)}
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
					<h3 class="section-heading">{t('hotspots.heading')}</h3>
					<span class="hint">{loading ? t('hotspots.searching') : searched ? t('hotspots.found', { count: recommendations.length }) : ''}</span>
				</div>
				{#if searched && recommendations.length === 0}
					<p class="hint padded">
						{t(targetSpecies.length > 0 ? 'hotspots.noneForBirds' : 'hotspots.none')}
					</p>
				{/if}
				<HotspotList
					hotspots={recommendations}
					selectedId={selectedHotspotId}
					birds={targetSpecies}
					onselect={selectHotspot}
					onpickspecies={openSpeciesDetail}
					onsearchhere={(hotspot) => moveHome(hotspot.latitude, hotspot.longitude, hotspot.hotspot_name)}
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
					<h3 class="section-heading">{seasonTitle}</h3>
					<span class="hint">{t('season.peaking')}</span>
				</div>
				{#if targetSpecies.length === 0 && seasonCount > 0}
					<p class="hint padded">{t('season.addHint')}</p>
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
					<p class="hint padded">{t('season.none')}</p>
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
					{@render yearScrubber()}
				{/if}
			</section>
		</aside>

		<div class="map-area">
			<div class="plate">
				<HotspotMap
					{home}
					hotspots={recommendations}
					selectedId={selectedHotspotId}
					birds={targetSpecies}
					layer={mapLayer}
					onselect={selectHotspot}
					onpick={(lat, lon) => moveHome(lat, lon, 'your point')}
				/>
			</div>
			<div class="caption">
				<p>
					{mapCaption}
					{t(mapLayer === 'yourBirds' ? 'map.bubbleBirds' : 'map.bubbleAll')}
					{#if targetSpecies.length > 0}{t('map.recentRing')}{/if}
					{t('map.clickToMove')}
				</p>
				<div class="layer-switch" role="group" aria-label={t('map.layer')}>
					<button type="button" class="text-button" aria-pressed={mapLayer === 'all'} onclick={() => (mapLayer = 'all')}>
						{t('map.all')}
					</button>
					<button
						type="button"
						class="text-button"
						aria-pressed={mapLayer === 'yourBirds'}
						disabled={targetSpecies.length === 0}
						onclick={() => (mapLayer = 'yourBirds')}
					>
						{t('common.yourBirds')}
					</button>
				</div>
			</div>
			{#if scrubberWeeks.length > 0}
				<div class="year-strip">
					{@render yearScrubber()}
				</div>
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
		width: 420px;
		padding-bottom: 1.5rem;
		overflow-y: auto;
		border-right: 1px solid var(--color-text);
		background: var(--color-ground);
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
		padding: 0;
		border: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 0.9rem;
		line-height: 1.4;
		text-align: left;
		cursor: pointer;
	}

	.area {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 20px 20px 16px;
	}

	.eyebrow {
		margin: 0;
		color: var(--color-muted);
		font-size: 0.85rem;
	}

	.place {
		margin: 0;
		font-size: 3rem;
		font-weight: 600;
		letter-spacing: -0.02em;
		line-height: 1.05;
		overflow-wrap: anywhere;
	}

	.date input {
		height: 36px;
		padding: 0 4px;
		border: 0;
		border-bottom: 1px solid var(--color-text);
		border-radius: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 500;
		font-variant-numeric: tabular-nums;
	}

	.link {
		align-self: flex-start;
		padding: 0;
		border: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 500;
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
		gap: 8px;
		padding: 16px 20px;
	}

	.row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 8px;
	}

	.count {
		color: var(--color-muted);
		font-size: 0.85rem;
		font-variant-numeric: tabular-nums;
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
		gap: 4px 16px;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.chips li {
		display: inline-flex;
		align-items: baseline;
		gap: 2px;
		font-size: 1rem;
		font-weight: 500;
	}

	.chips button {
		min-height: 32px;
		padding: 0;
		border: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		cursor: pointer;
	}

	.chips .name {
		overflow-wrap: anywhere;
		text-align: left;
		text-decoration: underline;
		text-decoration-color: var(--color-line);
		text-underline-offset: 4px;
	}

	.chips .remove {
		min-width: 24px;
		color: var(--color-muted);
		font-size: 1.2rem;
		line-height: 1;
	}

	.hotspots,
	.season {
		padding-top: 18px;
	}

	.hotspots .row,
	.season .row {
		padding: 0 20px 8px;
	}

	.tabs,
	.year-tab {
		display: none;
	}

	.map-area {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 12px;
		min-width: 0;
		padding: 24px 28px 20px;
	}

	/* The map is a plate: a hairline frame with a second line outside it. */
	.plate {
		position: relative;
		flex: 1;
		min-height: 200px;
		margin: 5px;
		border: 1px solid var(--color-text);
		outline: 1px solid var(--color-text);
		outline-offset: 4px;
	}

	.caption {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 4px 24px;
		padding: 4px 5px 0;
	}

	.caption p {
		flex: 1 1 360px;
		margin: 0;
		color: var(--color-muted);
		font-size: 0.8rem;
		line-height: 1.5;
	}

	.layer-switch {
		display: flex;
		gap: 16px;
		font-size: 0.85rem;
		font-weight: 500;
	}

	.year-strip {
		padding: 12px 5px 0;
		border-top: 1px solid var(--color-text);
	}

	@media (max-width: 639px) {
		.tabs {
			position: sticky;
			top: 0;
			z-index: 3;
			display: flex;
			padding: 0 8px;
			border-bottom: 1px solid var(--color-line);
			background: var(--color-ground);
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
			font-weight: 500;
			cursor: pointer;
		}

		.tabs button[aria-selected='true'] {
			border-bottom-color: var(--color-accent-strong);
			color: var(--color-text);
			font-weight: 600;
		}

		.place {
			font-size: 2.25rem;
		}

		.summary {
			display: flex;
			align-items: center;
			justify-content: space-between;
			min-height: 44px;
			padding: 0;
			border: 0;
			border-bottom: 1px solid var(--color-text);
			background: none;
			color: var(--color-text);
			font: inherit;
			font-weight: 500;
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

		.year-strip,
		.caption p {
			display: none;
		}

		.map-area {
			padding: 0;
		}

		.plate {
			position: static;
			margin: 0;
			border: 0;
			outline: 0;
		}

		.sidebar {
			position: fixed;
			right: 0;
			bottom: 0;
			left: 0;
			z-index: 10;
			width: auto;
			height: 45dvh;
			border-top: 1px solid var(--color-text);
			border-right: 0;
		}

		.link {
			min-height: 44px;
		}

		.season-link {
			display: none;
		}

		.layer-switch {
			position: fixed;
			top: 68px;
			right: 12px;
			z-index: 5;
			padding: 0 12px;
			background: var(--color-ground);
			box-shadow: 0 0 0 1px var(--color-text);
		}

		.layer-switch button {
			min-height: 44px;
		}
	}
</style>
