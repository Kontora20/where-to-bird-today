<script lang="ts">
	import type { Recommendation } from '$lib/api';
	import { checklistCount, distanceText } from '$lib/format';

	interface Props {
		hotspot: Recommendation;
		onclose: () => void;
		onpickspecies: (speciesId: number) => void;
		onsearchhere: () => void;
	}

	let { hotspot, onclose, onpickspecies, onsearchhere }: Props = $props();

	const shown = $derived(hotspot.top_species.slice(0, 5));
</script>

<section class="card" aria-label={hotspot.hotspot_name}>
	<div class="head">
		<div>
			<h2>{hotspot.hotspot_name}</h2>
			<p class="meta">{checklistCount(hotspot)} checklists in this week of the year · {distanceText(hotspot)}</p>
		</div>
		<button type="button" class="close" onclick={onclose} aria-label="Close">✕</button>
	</div>

	{#if shown.length > 0}
		<h3>Most likely this week</h3>
		<ul class="seen">
			{#each shown as species (species.species_id)}
				{@const percent = Math.round(species.frequency * 100)}
				<li>
					<button type="button" onclick={() => onpickspecies(species.species_id)}>
						<span class="name">{species.common_name}</span>
						<span class="track"><span class="fill" style="width: {percent}%"></span></span>
						<span class="percent">{percent}%</span>
					</button>
				</li>
			{/each}
		</ul>
		<p class="note">
			Share of that week's checklists that recorded the bird. Select a bird for its season and time of day.
		</p>
	{:else}
		<p class="note">No checklists at this hotspot in this week of the year.</p>
	{/if}

	<div class="actions">
		<button type="button" class="primary" onclick={onsearchhere}>Search around here</button>
		{#if hotspot.verification_url}
			<a href={hotspot.verification_url} target="_blank" rel="noopener noreferrer">eBird ↗</a>
		{/if}
	</div>
</section>

<style>
	.card {
		position: absolute;
		top: 16px;
		right: 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		width: 340px;
		max-height: calc(100% - 32px);
		padding: 16px;
		overflow-y: auto;
		border-radius: 12px;
		background: var(--color-panel);
		box-shadow: var(--shadow-raised);
	}

	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 8px;
	}

	h2 {
		margin: 0;
		font-size: 1.1rem;
	}

	h3 {
		margin: 0;
		color: var(--color-muted);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.05em;
		text-transform: uppercase;
	}

	.meta,
	.note {
		margin: 0;
		color: var(--color-muted);
		font-size: 0.8rem;
	}

	.close {
		flex-shrink: 0;
		width: 36px;
		height: 36px;
		border: 0;
		border-radius: 999px;
		background: var(--color-control-track);
		color: var(--color-text);
		font: inherit;
		cursor: pointer;
	}

	.seen {
		display: flex;
		flex-direction: column;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.seen button {
		display: flex;
		align-items: center;
		gap: 10px;
		width: 100%;
		padding: 5px 0;
		border: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 0.9rem;
		text-align: left;
		cursor: pointer;
	}

	.name {
		flex: 0 0 130px;
	}

	.track {
		flex: 1;
		height: 6px;
		border-radius: 3px;
		background: var(--color-line);
	}

	.fill {
		display: block;
		height: 6px;
		border-radius: 3px;
		background: var(--color-accent-strong);
	}

	.percent {
		width: 36px;
		text-align: right;
		font: 600 0.8rem var(--font-mono);
	}

	.actions {
		display: flex;
		gap: 8px;
	}

	.primary {
		flex: 1;
		height: 44px;
		border: 0;
		border-radius: 8px;
		background: var(--color-accent);
		color: var(--color-on-accent);
		font: inherit;
		font-weight: 700;
		cursor: pointer;
	}

	.actions a {
		display: flex;
		align-items: center;
		padding: 0 14px;
		border: 1px solid var(--color-control-border);
		border-radius: 8px;
		color: var(--color-accent-text);
		font-size: 0.9rem;
		font-weight: 600;
		text-decoration: none;
	}

	@media (max-width: 639px) {
		.card {
			position: fixed;
			inset: auto 0 0;
			z-index: 20;
			width: auto;
			min-height: 45dvh;
			max-height: 60dvh;
			border-radius: 20px 20px 0 0;
			box-shadow: var(--shadow-sheet);
		}
	}
</style>
