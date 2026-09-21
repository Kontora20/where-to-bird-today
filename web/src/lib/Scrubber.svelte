<script lang="ts">
	import { formatDay, type ScrubberWeek } from '$lib/format';

	interface Props {
		weeks: ScrubberWeek[];
		// The index in weeks of the week of the chosen date, or -1.
		selected: number;
		note: string;
		onpick: (start: string) => void;
		onshift: (weeks: number) => void;
		ontoday: () => void;
	}

	let { weeks, selected, note, onpick, onshift, ontoday }: Props = $props();

	const months = ['J', 'F', 'M', 'A', 'M', 'J', 'J', 'A', 'S', 'O', 'N', 'D'];
	const highest = $derived(Math.max(...weeks.map((week) => week.value)));
</script>

<section class="scrubber" aria-label="Slide through the year">
	<div class="head">
		<h2>Slide through the year</h2>
		<div class="buttons">
			<button type="button" class="step" aria-label="Previous week" onclick={() => onshift(-1)}>◀</button>
			<button type="button" class="step" aria-label="Next week" onclick={() => onshift(1)}>▶</button>
			<button type="button" onclick={ontoday}>Today</button>
		</div>
	</div>

	<div class="bars">
		{#each weeks as week, index (week.week)}
			<button
				type="button"
				class="bar"
				class:emphasized={week.emphasized}
				class:selected={index === selected}
				style="height: {highest > 0 ? Math.max(3, Math.round((week.value / highest) * 56)) : 3}px"
				title={week.title}
				aria-label="Week {week.week}, from {formatDay(week.start)}"
				aria-current={index === selected ? 'date' : undefined}
				onclick={() => onpick(week.start)}
			></button>
		{/each}
	</div>
	<div class="months" aria-hidden="true">
		{#each months as month, index (index)}
			<span>{month}</span>
		{/each}
	</div>

	<p class="note">
		{#if selected >= 0}<b>Week of {formatDay(weeks[selected].start)}.</b>{/if}
		<span>{note}</span>
	</p>
</section>

<style>
	.scrubber {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
	}

	h2 {
		margin: 0;
		font-size: 0.95rem;
	}

	.buttons {
		display: flex;
		gap: 6px;
	}

	.buttons button {
		height: 36px;
		padding: 0 12px;
		border: 1px solid var(--color-control-border);
		border-radius: 8px;
		background: var(--color-panel);
		color: var(--color-text);
		font: inherit;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}

	.buttons .step {
		width: 44px;
		padding: 0;
	}

	.bars {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 56px;
	}

	.bar {
		flex: 1;
		min-height: 3px;
		padding: 0;
		border: 0;
		border-radius: 2px 2px 0 0;
		background: var(--color-bar);
		cursor: pointer;
	}

	.bar.emphasized {
		background: var(--color-accent-strong);
	}

	.bar.selected {
		background: var(--color-text);
	}

	.months {
		display: flex;
		padding-top: 3px;
		border-top: 1px solid var(--color-line);
		color: var(--color-muted);
		font: 0.75rem var(--font-mono);
	}

	.months span {
		flex: 1;
	}

	.note {
		margin: 0;
		font-size: 0.8rem;
	}

	.note span {
		color: var(--color-muted);
	}
</style>
