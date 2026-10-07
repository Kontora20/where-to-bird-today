<script lang="ts">
	import { driveMinutes, radiusKm, rangeLabel, rangeLimits } from '$lib/format';

	interface Props {
		// "min:60" for a drive time of 60 minutes, "km:25" for 25 km in a straight line.
		value: string;
		// The place the range is measured from.
		place: string;
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
	<div class="toggle" role="group" aria-label="Measure the distance by">
		<button type="button" aria-pressed={unit === 'min'} onclick={() => setUnit('min')}>Drive time</button>
		<button type="button" aria-pressed={unit === 'km'} onclick={() => setUnit('km')}>Straight line</button>
	</div>
	<div class="amounts" role="group" aria-label={unit === 'min' ? 'Minutes of driving' : 'Kilometres'}>
		{#each amounts as option (option)}
			<button type="button" aria-pressed={!custom && option === amount} onclick={() => pick(option)}>
				{rangeLabel(unit, option)}
			</button>
		{/each}
		<button type="button" aria-pressed={custom} onclick={() => (customOpen = true)}>Custom</button>
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
			{unit === 'min' ? 'minutes' : 'km'}
			<span class="limits">({limits.low}–{limits.high})</span>
		</label>
	{/if}
	<p class="caption">
		{#if unit === 'min'}
			Hotspots up to {amount} minutes by car from {place}.
		{:else}
			Hotspots up to {amount} km from {place}, measured in a straight line.
		{/if}
	</p>
</div>

<style>
	.picker {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.toggle {
		display: flex;
		align-self: flex-start;
		padding: 3px;
		border-radius: 10px;
		background: var(--color-control-track);
	}

	.toggle button {
		height: 36px;
		padding: 0 14px;
		border: 0;
		border-radius: 8px;
		background: none;
		color: var(--color-muted);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 600;
		cursor: pointer;
	}

	.toggle button[aria-pressed='true'] {
		background: var(--color-inverse);
		color: var(--color-on-inverse);
	}

	.amounts {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.amounts button {
		height: 36px;
		padding: 0 12px;
		border: 1px solid var(--color-control-border);
		border-radius: 999px;
		background: var(--color-panel);
		color: var(--color-text);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 600;
		cursor: pointer;
	}

	.amounts button[aria-pressed='true'] {
		border-color: var(--color-inverse);
		background: var(--color-inverse);
		color: var(--color-on-inverse);
	}

	.custom {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.9rem;
		font-weight: 600;
	}

	.custom input {
		width: 90px;
		height: 36px;
		padding: 0 10px;
		border: 1px solid var(--color-control-border);
		border-radius: 10px;
		background: var(--color-panel);
		color: var(--color-text);
		font: inherit;
		font-family: var(--font-mono);
	}

	.limits {
		color: var(--color-muted);
		font-weight: 400;
	}

	.caption {
		margin: 0;
		color: var(--color-muted);
		font-size: 0.85rem;
	}

	@media (max-width: 639px) {
		.toggle button,
		.amounts button,
		.custom input {
			height: 44px;
		}
	}
</style>
