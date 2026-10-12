<script lang="ts">
	import { placeLabel, t } from '$lib/i18n.svelte';
	import { driveMinutes, radiusKm, rangeLabel, rangeLimits } from '$lib/format';

	interface Props {
		// "min:60" for a drive time of 60 minutes, "km:25" for 25 km in a straight line.
		value: string;
		// The place the range is measured from; when given, a sentence says so.
		place?: string;
		onchange: (value: string) => void;
	}

	let { value, place, onchange }: Props = $props();

	const unit = $derived(value.startsWith('km:') ? 'km' : 'min');
	const amount = $derived(Number(value.split(':')[1]));
	const amounts = $derived(unit === 'min' ? driveMinutes : radiusKm);

	let customOpen = $state(false);
	const custom = $derived(customOpen || !amounts.includes(amount));
	const limits = $derived(rangeLimits[unit]);

	function setUnit(next: 'min' | 'km') {
		if (next === unit) return;
		customOpen = false;
		onchange(next === 'min' ? 'min:60' : 'km:50');
	}

	function pick(option: number) {
		customOpen = false;
		onchange(`${unit}:${option}`);
	}

	// Runs when the box is left or Enter is pressed, so typing "75" does not
	// first search for 7.
	function setCustom(input: HTMLInputElement) {
		const typed = Math.round(Number(input.value));
		if (!Number.isFinite(typed) || input.value === '') {
			input.value = String(amount);
			return;
		}
		const clamped = Math.min(limits.high, Math.max(limits.low, typed));
		input.value = String(clamped);
		if (clamped !== amount) onchange(`${unit}:${clamped}`);
	}
</script>

<div class="picker">
	<div class="measure" role="group" aria-label={t('range.measureBy')}>
		<button type="button" class="text-button" aria-pressed={unit === 'min'} onclick={() => setUnit('min')}>
			{t('range.driveTime')}
		</button>
		<button type="button" class="text-button" aria-pressed={unit === 'km'} onclick={() => setUnit('km')}>
			{t('range.straightLine')}
		</button>
	</div>
	<div class="amounts" role="group" aria-label={t(unit === 'min' ? 'range.minutesGroup' : 'range.kilometresGroup')}>
		{#each amounts as option (option)}
			<button type="button" aria-pressed={!custom && option === amount} onclick={() => pick(option)}>
				{rangeLabel(unit, option)}
			</button>
		{/each}
		<button type="button" aria-pressed={custom} onclick={() => (customOpen = true)}>{t('range.custom')}</button>
	</div>
	{#if custom}
		<label class="custom">
			<input
				type="number"
				inputmode="numeric"
				min={limits.low}
				max={limits.high}
				step="1"
				value={amount}
				onchange={(event) => setCustom(event.currentTarget)}
			/>
			{t(unit === 'min' ? 'range.minutesUnit' : 'range.kmUnit')}
			<span class="limits">({limits.low}–{limits.high})</span>
		</label>
	{/if}
	{#if place}
		<p class="caption">
			{t(unit === 'min' ? 'range.captionDrive' : 'range.captionStraight', { amount, place: placeLabel(place) })}
		</p>
	{/if}
</div>

<style>
	.picker {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.measure {
		display: flex;
		gap: 16px;
	}

	.measure button {
		min-height: 36px;
		font-size: 0.9rem;
		font-weight: 500;
	}

	.amounts {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}

	/* The chosen amount is circled in ink. */
	.amounts button {
		min-width: 44px;
		height: 36px;
		padding: 0 12px;
		border: 1.5px solid transparent;
		border-radius: 999px;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 0.9rem;
		font-variant-numeric: tabular-nums;
		cursor: pointer;
	}

	.amounts button[aria-pressed='true'] {
		border-color: var(--color-text);
		font-weight: 600;
	}

	.custom {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.9rem;
		font-weight: 500;
	}

	.custom input {
		width: 80px;
		height: 36px;
		padding: 0 4px;
		border: 0;
		border-bottom: 1px solid var(--color-text);
		background: none;
		color: var(--color-text);
		font: inherit;
		font-variant-numeric: tabular-nums;
	}

	.caption {
		margin: 0;
		color: var(--color-muted);
		font-size: 0.85rem;
	}

	.limits {
		color: var(--color-muted);
		font-weight: 400;
	}

	@media (max-width: 639px) {
		.measure button,
		.amounts button,
		.custom input {
			min-height: 44px;
		}
	}
</style>
