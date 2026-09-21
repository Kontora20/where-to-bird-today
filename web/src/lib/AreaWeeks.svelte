<script lang="ts">
	import type { AreaWeeks } from '$lib/api';
	import { formatDay } from '$lib/format';

	interface Props {
		areaWeeks: AreaWeeks;
		speciesCount: number;
		onpickweek: (start: string) => void;
	}

	let { areaWeeks, speciesCount, onpickweek }: Props = $props();

	const highest = $derived(Math.max(...areaWeeks.weeks.map((week) => week.smoothed_frequency)));
</script>

<section>
	<h2>Best weeks for {speciesCount === 1 ? 'this bird' : 'these birds'} in this area</h2>
	<div class="seasonality">
		{#each areaWeeks.weeks as week (week.week)}
			{@const expected =
				speciesCount === 1
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
				onclick={() => onpickweek(week.start)}
			></button>
		{/each}
	</div>
	<p class="chart-note">
		{#if areaWeeks.best_weeks.length > 0}Best: {areaWeeks.best_weeks
				.map((range) => `${formatDay(range.start)} – ${formatDay(range.end)}`)
				.join(', ')}.{/if}
		Each week is pooled with the weeks either side across the hotspots in range. The outlined week is the one searched; select a
		week to search it.
	</p>
</section>

<style>
	h2 {
		margin: 0 0 0.5rem;
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.05em;
		text-transform: uppercase;
		color: var(--color-muted);
	}
</style>
