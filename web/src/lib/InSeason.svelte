<script lang="ts">
	import type { SeasonalSpecies, SpeciesMatch } from '$lib/api';
	import { peakText, sparkBars } from '$lib/format';

	interface Props {
		species: SeasonalSpecies[];
		// The week of the year the list is for.
		week: number;
		birds: SpeciesMatch[];
		// Your birds is at its limit.
		full: boolean;
		ontoggle: (species: SeasonalSpecies) => void;
		onopen: (speciesId: number) => void;
	}

	let { species, week, birds, full, ontoggle, onopen }: Props = $props();
</script>

<ul class="list">
	{#each species as bird (bird.species_id)}
		{@const tracked = birds.some((mine) => mine.species_id === bird.species_id)}
		<li>
			<span class="photo" aria-hidden="true"></span>
			<button type="button" class="info" onclick={() => onopen(bird.species_id)}>
				<span class="name">{bird.common_name}</span>
				<span class="note">{peakText(bird)}</span>
			</button>
			<span class="spark" aria-hidden="true" title="Chance of seeing it through the year">
				{#each sparkBars(bird.year, week) as bar, index (index)}
					<span class={bar.kind} style="height: {bar.height}px"></span>
				{/each}
			</span>
			<button
				type="button"
				class="toggle"
				class:tracked
				aria-pressed={tracked}
				aria-label={tracked ? `Remove ${bird.common_name} from Your birds` : `Add ${bird.common_name} to Your birds`}
				disabled={!tracked && full}
				onclick={() => ontoggle(bird)}
			>
				{tracked ? '✓' : '+'}
			</button>
		</li>
	{/each}
</ul>

<style>
	.list {
		display: flex;
		flex-direction: column;
		margin: 0;
		padding: 0 12px;
		list-style: none;
	}

	li {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 6px 8px;
		border-top: 1px solid var(--color-line-soft);
	}

	.photo {
		flex-shrink: 0;
		width: 40px;
		height: 40px;
		border-radius: 8px;
		background: repeating-linear-gradient(
			45deg,
			var(--color-bar),
			var(--color-bar) 6px,
			var(--color-line-soft) 6px,
			var(--color-line-soft) 12px
		);
	}

	.info {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-width: 0;
		padding: 0;
		border: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		text-align: left;
		cursor: pointer;
	}

	.name {
		font-size: 0.9rem;
		font-weight: 600;
	}

	.note {
		color: var(--color-muted);
		font-size: 0.8rem;
	}

	.spark {
		display: flex;
		align-items: flex-end;
		gap: 1px;
		flex-shrink: 0;
		width: 64px;
		height: 28px;
	}

	.spark span {
		flex: 1;
		border-radius: 1px 1px 0 0;
		background: var(--color-bar);
	}

	.spark .high {
		background: var(--color-accent-strong);
	}

	.spark .now {
		background: var(--color-text);
	}

	.toggle {
		flex-shrink: 0;
		width: 40px;
		height: 40px;
		border: 1px solid var(--color-control-border);
		border-radius: 999px;
		background: var(--color-panel);
		color: var(--color-text);
		font: 700 1.1rem/1 inherit;
		cursor: pointer;
	}

	.toggle.tracked {
		border-color: var(--color-accent-strong);
		background: var(--color-chip);
	}

	.toggle:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
