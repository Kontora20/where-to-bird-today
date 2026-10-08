<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import type { SpeciesMatch } from '$lib/api';
	import { placeLabel, t } from '$lib/i18n.svelte';
	import { cities, type Place } from '$lib/cities';
	import { dateLimits, formatDay, isUsableDate, nextSaturday, todayIso } from '$lib/format';
	import RangePicker from '$lib/RangePicker.svelte';
	import SpeciesSearch from '$lib/SpeciesSearch.svelte';

	export interface Plan {
		home: Place;
		range: string;
		date: string;
		birds: SpeciesMatch[];
	}

	interface Props {
		home: Place;
		range: string;
		date: string;
		birds: SpeciesMatch[];
		maxBirds: number;
		onapply: (plan: Plan) => void;
		onclose: () => void;
	}

	let { home, range, date, birds, maxBirds, onapply, onclose }: Props = $props();

	// The panel is mounted afresh each time it opens, so the draft starts from
	// the page's values once and nothing the page does later changes it.
	let start = $state(untrack(() => home));
	let draftRange = $state(untrack(() => range));
	let day = $state(untrack(() => date));
	let draftBirds = $state(untrack(() => [...birds]));
	let otherDate = $state(false);
	let locating = $state(false);
	let error = $state('');
	let panel: HTMLElement;

	// The page behind is inert while this is open, so focus moves in here and,
	// on closing, back to what opened it.
	onMount(() => {
		const opener = document.activeElement;
		panel.focus();
		return () => setTimeout(() => opener instanceof HTMLElement && opener.focus());
	});

	const today = todayIso();
	const saturday = nextSaturday(today);
	const startKey = (place: Place) => `${place.lat},${place.lon}`;
	const knownCity = $derived(cities.some((city) => startKey(city) === startKey(start)));
	const showOtherDate = $derived(otherDate || (day !== today && day !== saturday));

	function pickCity(key: string) {
		const city = cities.find((candidate) => startKey(candidate) === key);
		if (city) start = city;
	}

	function useMyLocation() {
		if (!navigator.geolocation) {
			error = t('error.noGeolocation');
			return;
		}
		error = '';
		locating = true;
		navigator.geolocation.getCurrentPosition(
			(position) => {
				locating = false;
				start = { lat: position.coords.latitude, lon: position.coords.longitude, label: 'you' };
			},
			(geoError) => {
				locating = false;
				error = geoError.message;
			}
		);
	}

	function pickDay(next: string) {
		day = next;
		otherDate = false;
	}

	function addBird(species: SpeciesMatch) {
		if (draftBirds.length >= maxBirds || draftBirds.some((bird) => bird.species_id === species.species_id)) return;
		draftBirds = [...draftBirds, species];
	}

	function removeBird(speciesId: number) {
		draftBirds = draftBirds.filter((bird) => bird.species_id !== speciesId);
	}

	function submit(event: SubmitEvent) {
		event.preventDefault();
		onapply({ home: start, range: draftRange, date: isUsableDate(day) ? day : today, birds: draftBirds });
	}
</script>

<svelte:window
	onkeydown={(event) => {
		if (event.key === 'Escape' && !event.defaultPrevented) onclose();
	}}
/>

<div class="backdrop" onclick={(event) => event.target === event.currentTarget && onclose()} role="presentation">
	<div class="panel" role="dialog" aria-modal="true" aria-labelledby="plan-title" tabindex="-1" bind:this={panel}>
		<form onsubmit={submit}>
			<header>
				<h2 id="plan-title">{t('plan.title')}</h2>
				<button type="button" class="close" onclick={onclose} aria-label={t('common.close')}>✕</button>
			</header>

			<div class="fields">
				<div class="field">
					<label for="plan-start">{t('plan.start')}</label>
					<div class="start">
						<select id="plan-start" value={startKey(start)} onchange={(event) => pickCity(event.currentTarget.value)}>
							{#if !knownCity}
								<option value={startKey(start)}>{placeLabel(start.label)}</option>
							{/if}
							{#each cities as city (city.label)}
								<option value={startKey(city)}>{city.label}</option>
							{/each}
						</select>
						<button type="button" class="square" onclick={useMyLocation} disabled={locating} aria-label={t('plan.useLocation')}>
							<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
								<circle cx="12" cy="12" r="4" />
								<path d="M12 2v4M12 18v4M2 12h4M18 12h4" />
							</svg>
						</button>
					</div>
					<p class="hint">{t('plan.hint')}</p>
					{#if error}
						<p class="error" role="alert">{error}</p>
					{/if}
				</div>

				<div class="field">
					<span class="label">{t('plan.howFar')}</span>
					<RangePicker value={draftRange} place={start.label} onchange={(next) => (draftRange = next)} />
				</div>

				<div class="field">
					<span class="label">{t('plan.when')}</span>
					<div class="days">
						<button type="button" aria-pressed={!showOtherDate && day === today} onclick={() => pickDay(today)}>{t('common.today')}</button>
						<button type="button" aria-pressed={!showOtherDate && day === saturday} onclick={() => pickDay(saturday)}>
							{t('plan.saturday', { day: formatDay(saturday) })}
						</button>
						<button type="button" aria-pressed={showOtherDate} onclick={() => (otherDate = true)}>{t('plan.other')}</button>
					</div>
					{#if showOtherDate}
						<input
							type="date"
							class="date"
							aria-label={t('common.date')}
							min={dateLimits.min}
							max={dateLimits.max}
							bind:value={day}
						/>
					{/if}
				</div>

				<div class="field">
					<div class="row">
						<span class="label">{t('plan.lookingFor')}</span>
						<span class="count">{draftBirds.length} / {maxBirds}</span>
					</div>
					{#if draftBirds.length > 0}
						<ul class="chips">
							{#each draftBirds as bird (bird.species_id)}
								<li>
									{bird.common_name}
									<button type="button" onclick={() => removeBird(bird.species_id)} aria-label={t('common.remove', { name: bird.common_name })}>✕</button>
								</li>
							{/each}
						</ul>
					{/if}
					<SpeciesSearch
						placeholder={t(draftBirds.length >= maxBirds ? 'search.limit' : 'search.placeholder')}
						disabled={draftBirds.length >= maxBirds}
						onpick={addBird}
						onerror={(message) => (error = message)}
					/>
				</div>
			</div>

			<footer>
				<button type="button" class="cancel" onclick={onclose}>{t('common.cancel')}</button>
				<button type="submit" class="search">{t('plan.search')}</button>
			</footer>
		</form>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		z-index: 30;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--color-backdrop);
	}

	.panel {
		width: min(480px, 100%);
		max-height: 100dvh;
		overflow-y: auto;
		border-radius: 16px;
		background: var(--color-ground);
		color: var(--color-text);
		box-shadow: var(--shadow-raised);
		outline: none;
	}

	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 20px 4px;
	}

	h2 {
		margin: 0;
		font-size: 1.25rem;
	}

	.close {
		width: 44px;
		height: 44px;
		border: 0;
		border-radius: 999px;
		background: none;
		color: var(--color-text);
		font: inherit;
		cursor: pointer;
	}

	.fields {
		display: flex;
		flex-direction: column;
		gap: 18px;
		padding: 12px 20px 20px;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	label,
	.label {
		color: var(--color-muted);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.05em;
		text-transform: uppercase;
	}

	.row,
	.start,
	.days {
		display: flex;
		gap: 8px;
	}

	.row {
		align-items: center;
		justify-content: space-between;
	}

	.count {
		color: var(--color-muted);
		font-family: var(--font-mono);
		font-size: 0.9rem;
	}

	select,
	.date,
	.square {
		height: 48px;
		border: 1px solid var(--color-control-border);
		border-radius: 10px;
		background: var(--color-panel);
		color: var(--color-text);
		font: inherit;
		font-size: 1rem;
	}

	select {
		flex: 1;
		min-width: 0;
		padding: 0 14px;
	}

	.date {
		padding: 0 14px;
	}

	.square {
		display: flex;
		flex-shrink: 0;
		align-items: center;
		justify-content: center;
		width: 48px;
		cursor: pointer;
	}

	.square:disabled {
		opacity: 0.6;
		cursor: wait;
	}

	.hint,
	.error {
		margin: 0;
		font-size: 0.85rem;
	}

	.hint {
		color: var(--color-muted);
	}

	.error {
		color: var(--color-error);
	}

	.days button {
		height: 44px;
		padding: 0 16px;
		border: 1px solid var(--color-control-border);
		border-radius: 999px;
		background: var(--color-panel);
		color: var(--color-text);
		font: inherit;
		font-weight: 600;
		cursor: pointer;
	}

	.days button[aria-pressed='true'] {
		border-color: var(--color-inverse);
		background: var(--color-inverse);
		color: var(--color-on-inverse);
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
		padding: 4px 4px 4px 14px;
		border-radius: 999px;
		background: var(--color-chip);
		font-weight: 600;
	}

	.chips button {
		width: 36px;
		height: 36px;
		border: 0;
		border-radius: 999px;
		background: none;
		color: var(--color-text);
		font: inherit;
		cursor: pointer;
	}

	footer {
		position: sticky;
		bottom: 0;
		display: flex;
		gap: 8px;
		padding: 12px 20px 20px;
		border-top: 1px solid var(--color-line);
		background: var(--color-ground);
	}

	footer button {
		height: 52px;
		border-radius: 10px;
		font: inherit;
		font-size: 1.05rem;
		font-weight: 700;
		cursor: pointer;
	}

	.cancel {
		padding: 0 20px;
		border: 1px solid var(--color-control-border);
		background: var(--color-panel);
		color: var(--color-text);
	}

	.search {
		flex: 1;
		border: 0;
		background: var(--color-accent);
		color: var(--color-on-accent);
	}

	@media (max-width: 639px) {
		.backdrop {
			align-items: flex-end;
		}

		.panel {
			border-radius: 20px 20px 0 0;
		}
	}
</style>
