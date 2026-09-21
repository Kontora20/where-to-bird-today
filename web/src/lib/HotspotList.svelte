<script lang="ts">
	import type { Recommendation } from '$lib/api';
	import { checklistCount, distanceText } from '$lib/format';

	interface Props {
		hotspots: Recommendation[];
		selectedId: number | null;
		birdCount: number;
		onselect: (hotspotId: number) => void;
	}

	let { hotspots, selectedId, birdCount, onselect }: Props = $props();
</script>

<ul class="list">
	{#each hotspots as hotspot (hotspot.hotspot_id)}
		<li id="hotspot-{hotspot.hotspot_id}">
			<button
				type="button"
				class:selected={hotspot.hotspot_id === selectedId}
				aria-pressed={hotspot.hotspot_id === selectedId}
				onclick={() => onselect(hotspot.hotspot_id)}
			>
				<span class="ring"></span>
				<span class="text">
					<span class="top">
						<span class="name">{hotspot.hotspot_name}</span>
						<span class="count" title="Checklists in this week of the year">{checklistCount(hotspot)}</span>
					</span>
					<span class="meta">
						{distanceText(hotspot)}{#if birdCount > 0}
							· {hotspot.score.toFixed(1)} of {birdCount} of your birds expected{/if}
					</span>
				</span>
			</button>
		</li>
	{/each}
</ul>

<style>
	.list {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin: 0;
		padding: 0 12px;
		list-style: none;
	}

	button {
		display: flex;
		gap: 12px;
		width: 100%;
		padding: 9px 8px;
		border: 1px solid var(--color-line);
		border-radius: 10px;
		background: var(--color-panel);
		color: var(--color-text);
		font: inherit;
		text-align: left;
		cursor: pointer;
	}

	button.selected {
		border-color: var(--color-accent-strong);
		background: var(--color-selected);
	}

	.ring {
		flex-shrink: 0;
		width: 22px;
		height: 22px;
		margin-top: 2px;
		border: 3px solid var(--color-accent-strong);
		border-radius: 999px;
		background: var(--color-panel);
	}

	.text {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 3px;
		min-width: 0;
	}

	.top {
		display: flex;
		justify-content: space-between;
		gap: 8px;
	}

	.name {
		font-size: 0.95rem;
		font-weight: 700;
	}

	.count {
		font: 600 0.8rem var(--font-mono);
	}

	.meta {
		color: var(--color-muted);
		font-size: 0.8rem;
	}
</style>
