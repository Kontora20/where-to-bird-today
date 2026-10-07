<script lang="ts">
	import { onMount } from 'svelte';
	import type { HourFrequency, SpeciesDetail } from '$lib/api';
	import { formatDay } from '$lib/format';

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
		if (event.key === 'Escape') onclose();
	}}
/>

<div
	class="backdrop"
	onclick={(event) => event.target === event.currentTarget && onclose()}
	role="presentation"
>
	<div class="panel" role="dialog" aria-modal="true" aria-label={species?.common_name ?? 'Bird details'} tabindex="-1" bind:this={panel}>
		<button type="button" class="close" onclick={onclose} aria-label="Close">✕</button>

		{#if loading}
			<p>Loading…</p>
		{:else if error}
			<p class="error" role="alert">{error}</p>
		{:else if species}
			<h2>{species.common_name}</h2>
			<p class="scientific-name">{species.scientific_name}</p>
			<p class="where">Figures for {hotspotName}</p>

			{#if species.photo}
				<figure>
					<img src={species.photo.image_url} alt={species.common_name} />
					<figcaption>
						{#if species.photo.attribution_url}
							<a href={species.photo.attribution_url} target="_blank" rel="noopener noreferrer">
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
				<h3>Seasonality at this hotspot</h3>
				<div class="seasonality">
					{#each species.seasonality as week (week.week)}
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
					{#if species.best_weeks.length > 0}Best: {species.best_weeks
							.map((range) => `${formatDay(range.start)} – ${formatDay(range.end)}`)
							.join(', ')}.{/if}
					Each week is pooled with the weeks either side; bar height is relative to the best week.
				</p>
			{/if}

			{#if species.time_of_day.length > 0}
				{@const highest = Math.max(...species.time_of_day.map((hour) => hour.adjusted_frequency))}
				{@const best = bestHours(species.time_of_day, highest)}
				<h3>Time of day at this hotspot</h3>
				<div class="time-of-day">
					{#each hoursAfterSunrise as hourAfterSunrise (hourAfterSunrise)}
						{@const match = species.time_of_day.find((hour) => hour.hours_after_sunrise === hourAfterSunrise)}
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
						url: species.latest_checklist?.url,
						label: `Latest record at this hotspot (${species.latest_checklist?.observed_on})`
					},
					{ url: species.ebird_url, label: 'eBird' },
					{ url: species.birds_of_the_world_url, label: 'Birds of the World' },
					{ url: species.birdlife_datazone_url, label: 'BirdLife DataZone' }
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
