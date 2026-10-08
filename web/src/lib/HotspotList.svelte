<script lang="ts">
	import { tick } from 'svelte';
	import type { Recommendation, SpeciesMatch } from '$lib/api';
	import { formatNumber, t } from '$lib/i18n.svelte';
	import { checklistCount, distanceText, lastSeenStatus } from '$lib/format';

	interface Props {
		hotspots: Recommendation[];
		selectedId: number | null;
		birds: SpeciesMatch[];
		onselect: (hotspotId: number) => void;
	}

	let { hotspots, selectedId, birds, onselect }: Props = $props();

	// The list starts with the best few so it does not push the sections below
	// it off the screen; the rest are one click away.
	const initiallyShown = 5;

	const today = new Date();
	let showAll = $state(false);
	const shown = $derived(showAll ? hotspots : hotspots.slice(0, initiallyShown));

	// A hotspot picked on the map may be one the list has not shown yet.
	$effect(() => {
		const index = hotspots.findIndex((hotspot) => hotspot.hotspot_id === selectedId);
		if (index < 0) return;
		if (index >= initiallyShown) showAll = true;
		void tick().then(() =>
			document.getElementById(`hotspot-${selectedId}`)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
		);
	});
</script>

<ul class="list">
	{#each shown as hotspot (hotspot.hotspot_id)}
		{@const status = lastSeenStatus(hotspot, birds, today)}
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
						<span class="count" title={t('list.countTitle')}>{t('list.count', { count: checklistCount(hotspot) })}</span>
					</span>
					<span class="meta">
						{distanceText(hotspot)}{#if birds.length > 0}
							· {t('list.expected', { score: formatNumber(hotspot.score, 1), count: birds.length })}{/if}
					</span>
					{#if status}
						<span class="status" class:recent={status.recent}>
							<span class="dot"></span>{status.text}
						</span>
					{/if}
				</span>
			</button>
		</li>
	{/each}
</ul>
{#if hotspots.length > initiallyShown}
	<button type="button" class="more" onclick={() => (showAll = !showAll)}>
		{showAll ? t('list.showFewer') : t('list.showMore', { count: hotspots.length - initiallyShown })}
	</button>
{/if}

<style>
	.list {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin: 0;
		padding: 0 12px;
		list-style: none;
	}

	.list button {
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

	.list button.selected {
		border-color: var(--color-accent-strong);
		background: var(--color-selected);
	}

	.more {
		margin: 6px 12px 0;
		padding: 8px 0;
		border: 0;
		background: none;
		color: var(--color-accent-text);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 600;
		text-decoration: underline;
		cursor: pointer;
	}

	@media (max-width: 639px) {
		.more {
			min-height: 44px;
		}
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

	.status {
		display: flex;
		align-items: center;
		gap: 6px;
		color: var(--color-muted);
		font-size: 0.8rem;
		font-weight: 600;
	}

	.status.recent {
		color: var(--color-accent-text);
	}

	.dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: 999px;
		background: var(--color-control-border);
	}

	.recent .dot {
		background: var(--color-accent-strong);
	}
</style>
