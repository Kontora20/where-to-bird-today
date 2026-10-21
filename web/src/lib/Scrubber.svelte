<script lang="ts">
	import { monthInitials, t } from '$lib/i18n.svelte';
	import { formatDay, type ScrubberWeek } from '$lib/format';

	interface Props {
		weeks: ScrubberWeek[];
		// The index in weeks of the week of the chosen date, or -1.
		selected: number;
		note: string;
		onpick: (start: string) => void;
		// Called for each new week while a finger or the mouse drags across the bars.
		onscrub: (start: string) => void;
		onshift: (weeks: number) => void;
		ontoday: () => void;
	}

	let { weeks, selected, note, onpick, onscrub, onshift, ontoday }: Props = $props();

	const months = $derived(monthInitials());
	const highest = $derived(Math.max(...weeks.map((week) => week.value)));

	function scrub(event: PointerEvent & { currentTarget: HTMLElement }) {
		if (!(event.buttons & 1)) return;
		const { left, width } = event.currentTarget.getBoundingClientRect();
		const index = Math.min(weeks.length - 1, Math.max(0, Math.floor(((event.clientX - left) / width) * weeks.length)));
		if (index !== selected) onscrub(weeks[index].start);
	}
</script>

<section class="scrubber" aria-label={t('year.label')}>
	<div class="head">
		<h2 class="section-heading">{t('year.label')}</h2>
		<div class="buttons">
			<button type="button" class="text-button step" aria-label={t('year.previous')} onclick={() => onshift(-1)}>◀</button>
			<button type="button" class="text-button step" aria-label={t('year.next')} onclick={() => onshift(1)}>▶</button>
			<button type="button" class="text-button today" onclick={ontoday}>{t('common.today')}</button>
		</div>
	</div>

	<div class="bars" onpointermove={scrub} role="presentation">
		{#each weeks as week, index (week.week)}
			<button
				type="button"
				class="bar"
				class:emphasized={week.emphasized}
				class:selected={index === selected}
				style="height: {highest > 0 ? Math.max(3, Math.round((week.value / highest) * 56)) : 3}px"
				title={week.title}
				aria-label={t('year.weekFrom', { week: week.week, day: formatDay(week.start) })}
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
		{#if selected >= 0}<b>{t('year.weekOf', { day: formatDay(weeks[selected].start) })}</b>{/if}
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
		align-items: baseline;
		justify-content: space-between;
		gap: 8px;
	}

	.buttons {
		display: flex;
		align-items: center;
		gap: 4px;
	}

	.buttons button {
		min-width: 44px;
		height: 44px;
		color: var(--color-text);
		font-size: 0.9rem;
		font-weight: 500;
	}

	.buttons .today {
		padding: 0 8px;
		text-decoration: underline;
	}

	.bars {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 56px;
		/* Horizontal drags select weeks; vertical drags still scroll the page. */
		touch-action: pan-y;
		user-select: none;
	}

	.bar {
		flex: 1;
		min-height: 3px;
		padding: 0;
		border: 0;
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
		font-size: 0.75rem;
		font-weight: 500;
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
