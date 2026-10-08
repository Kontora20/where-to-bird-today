<script lang="ts">
	import { onMount } from 'svelte';
	import type { HourFrequency, SpeciesDetail } from '$lib/api';
	import { monthInitials, t } from '$lib/i18n.svelte';
	import { formatDay, httpsUrl } from '$lib/format';

	interface Props {
		loading: boolean;
		error: string;
		species: SpeciesDetail | null;
		// The hotspot the seasonality and time of day are for.
		hotspotName: string;
		onclose: () => void;
	}

	let { loading, error, species, hotspotName, onclose }: Props = $props();

	let panel: HTMLElement;

	// The page behind is inert while this is open, so focus moves in here and,
	// on closing, back to what opened it.
	onMount(() => {
		const opener = document.activeElement;
		panel.focus();
		return () => setTimeout(() => opener instanceof HTMLElement && opener.focus());
	});

	const hoursAfterSunrise = Array.from({ length: 24 }, (_, index) => index);
	const months = $derived(monthInitials());

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
</script>

<svelte:window
	onkeydown={(event) => {
		if (event.key === 'Escape' && !event.defaultPrevented) onclose();
	}}
/>

<div
	class="backdrop"
	onclick={(event) => event.target === event.currentTarget && onclose()}
	role="presentation"
>
	<div class="panel" role="dialog" aria-modal="true" aria-label={species?.common_name ?? t('species.details')} tabindex="-1" bind:this={panel}>
		<button type="button" class="close" onclick={onclose} aria-label={t('common.close')}>✕</button>

		{#if loading}
			<p>{t('common.loading')}</p>
		{:else if error}
			<p class="error" role="alert">{error}</p>
		{:else if species}
			<h2>{species.common_name}</h2>
			<p class="scientific-name" lang="la">{species.scientific_name}</p>
			<p class="where">{t('species.figures', { hotspot: hotspotName })}</p>

			{#if species.photo}
				<figure>
					<img src={species.photo.image_url} alt={species.common_name} />
					<figcaption>
						{#if httpsUrl(species.photo.attribution_url)}
							<a href={httpsUrl(species.photo.attribution_url)} target="_blank" rel="noopener noreferrer">
								{species.photo.attribution}
							</a>
						{:else}
							{species.photo.attribution}
						{/if}
						· {species.photo.license}
					</figcaption>
				</figure>
			{/if}

			{#if species.seasonality.length > 0}
				{@const highest = Math.max(...species.seasonality.map((week) => week.smoothed_frequency))}
				<h3>{t('species.seasonality')}</h3>
				<div class="seasonality">
					{#each species.seasonality as week (week.week)}
						<div
							class="week-bar"
							class:best={week.best}
							style="height: {highest > 0 ? (week.smoothed_frequency / highest) * 100 : 0}%"
							title="{t('species.week', { week: week.week })}: {t('species.expected', {
								percent: (week.smoothed_frequency * 100).toFixed(0),
								detail:
									week.checklists === 0
										? t('species.noChecklists')
										: t('species.shareOf', { percent: (week.frequency * 100).toFixed(0), count: week.checklists })
							})}"
						></div>
					{/each}
				</div>
				<div class="axis months" aria-hidden="true">
					{#each months as month, index (index)}
						<span>{month}</span>
					{/each}
				</div>
				<ul class="legend">
					<li><span class="swatch best"></span>{t('species.bestWeeks')}</li>
					<li><span class="swatch"></span>{t('species.otherWeeks')}</li>
				</ul>
				<p class="chart-note">
					{#if species.best_weeks.length > 0}{t('species.best', {
							ranges: species.best_weeks.map((range) => `${formatDay(range.start)} – ${formatDay(range.end)}`).join(', ')
						})}{/if}
					{t('species.weeksNote')}
				</p>
			{/if}

			{#if species.time_of_day.length > 0}
				{@const highest = Math.max(...species.time_of_day.map((hour) => hour.adjusted_frequency))}
				{@const best = bestHours(species.time_of_day, highest)}
				<h3>{t('species.timeOfDay')}</h3>
				<div class="time-of-day">
					{#each hoursAfterSunrise as hourAfterSunrise (hourAfterSunrise)}
						{@const match = species.time_of_day.find((hour) => hour.hours_after_sunrise === hourAfterSunrise)}
						<div
							class="hour-bar"
							class:best={match && best.includes(match)}
							style="height: {match && highest > 0 ? (match.adjusted_frequency / highest) * 100 : 0}%"
							title={match
								? `${match.start}–${match.end}: ${t('species.expected', {
										percent: (match.adjusted_frequency * 100).toFixed(0),
										detail: t('species.shareOf', { percent: (match.frequency * 100).toFixed(0), count: match.checklists })
									})}`
								: t('species.noData')}
						></div>
					{/each}
				</div>
				<div class="axis hours" aria-hidden="true">
					<span>{t('species.sunrise')}</span>
					<span>{t('species.hoursAfter', { hours: 6 })}</span>
					<span>{t('species.hoursAfter', { hours: 12 })}</span>
					<span>{t('species.hoursAfter', { hours: 18 })}</span>
					<span>{t('species.sunrise')}</span>
				</div>
				<ul class="legend">
					<li><span class="swatch best"></span>{t('species.bestHours')}</li>
					<li><span class="swatch"></span>{t('species.otherHours')}</li>
				</ul>
				<p class="chart-note">
					{#if best.length > 0}{t('species.best', { ranges: clockRanges(best).join(', ') })}{/if}
					{t('species.hoursNote')}
				</p>
			{/if}

			<ul class="links">
				{#each [
					{
						url: httpsUrl(species.latest_checklist?.url),
						label: t('species.latest', { date: species.latest_checklist?.observed_on ?? '' })
					},
					{ url: httpsUrl(species.ebird_url), label: 'eBird' },
					{ url: httpsUrl(species.birds_of_the_world_url), label: 'Birds of the World' },
					{ url: httpsUrl(species.birdlife_datazone_url), label: 'BirdLife DataZone' }
				] as link (link.label)}
					{#if link.url}
						<li><a href={link.url} target="_blank" rel="noopener noreferrer">{link.label}</a></li>
					{/if}
				{/each}
			</ul>
		{/if}
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		display: flex;
		justify-content: flex-end;
		background: var(--color-backdrop);
		z-index: 1000;
	}

	.panel {
		position: relative;
		width: min(420px, 100%);
		height: 100%;
		overflow-y: auto;
		padding: 1.5rem;
		background: var(--color-panel);
		border-left: 1px solid var(--color-line);
		outline: none;
	}

	.close {
		position: absolute;
		top: 1rem;
		right: 1rem;
		border: none;
		background: none;
		font-size: 1.1rem;
		cursor: pointer;
		color: var(--color-text);
	}

	.error {
		color: var(--color-error);
	}

	h2 {
		margin: 0 0 0.15rem;
		overflow-wrap: anywhere;
	}

	h3 {
		font-size: 0.95rem;
		margin: 1rem 0 0.5rem;
	}

	.scientific-name {
		margin: 0 0 1rem;
		font-style: italic;
		color: var(--color-muted);
	}

	.where {
		margin: -0.5rem 0 1rem;
		color: var(--color-muted);
		font-size: 0.85rem;
	}

	figure {
		margin: 0 0 1rem;
	}

	img {
		width: 100%;
		border-radius: 8px;
	}

	figcaption {
		margin-top: 0.3rem;
		font-size: 0.75rem;
		color: var(--color-muted);
	}

	.time-of-day {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 80px;
		border-bottom: 1px solid var(--color-line);
	}

	.hour-bar {
		flex: 1;
		min-height: 2px;
		background: var(--color-bar);
		border-radius: 2px 2px 0 0;
	}

	.hour-bar.best {
		background: var(--color-accent-strong);
	}

	.axis {
		display: flex;
		margin-top: 2px;
		color: var(--color-muted);
		font: 0.7rem var(--font-mono);
	}

	.axis.months span {
		flex: 1;
		text-align: center;
	}

	.axis.hours {
		justify-content: space-between;
	}

	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 4px 14px;
		margin: 6px 0 0;
		padding: 0;
		list-style: none;
		color: var(--color-muted);
		font-size: 0.75rem;
	}

	.legend li {
		display: flex;
		align-items: center;
		gap: 6px;
	}

	.swatch {
		width: 12px;
		height: 12px;
		border-radius: 3px;
		background: var(--color-bar);
	}

	.swatch.best {
		background: var(--color-accent-strong);
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
		color: var(--color-accent-text);
	}

	@media (max-width: 639px) {
		.close {
			top: 0.5rem;
			right: 0.5rem;
			width: 44px;
			height: 44px;
		}

		.links a {
			display: flex;
			align-items: center;
			min-height: 44px;
		}
	}
</style>
