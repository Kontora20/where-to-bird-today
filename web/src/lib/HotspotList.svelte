<script lang="ts">
	import { tick } from 'svelte';
	import type { Recommendation, SpeciesMatch } from '$lib/api';
	import { t } from '$lib/i18n.svelte';
	import {
		chanceKey,
	expectedKey,
		checklistCount,
		distanceText,
		fewChecklists,
		httpsUrl,
		lastSeenStatus,
		sighting,
		splitName
	} from '$lib/format';

	interface Props {
		hotspots: Recommendation[];
		selectedId: number | null;
		birds: SpeciesMatch[];
		// Null when the open entry is picked again and closes.
		onselect: (hotspotId: number | null) => void;
		onpickspecies: (hotspot: Recommendation, speciesId: number) => void;
		onsearchhere: (hotspot: Recommendation) => void;
	}

	let { hotspots, selectedId, birds, onselect, onpickspecies, onsearchhere }: Props = $props();

	// The list starts with the best few so it does not push the sections below
	// it off the screen; the rest are one click away.
	const initiallyShown = 5;
	const step = 5;

	const today = new Date();
	let visible = $state(initiallyShown);
	// A hotspot picked on the map may be one the list has not shown yet: it is
	// put after the last visible row, keeping its own number.
	const pinned = $derived(hotspots.findIndex((hotspot) => hotspot.hotspot_id === selectedId));
	const shown = $derived(
		hotspots
			.map((hotspot, index) => ({ hotspot, rank: index + 1 }))
			.filter((_, index) => index < visible || index === pinned)
	);
	const hidden = $derived(hotspots.length - shown.length);

	$effect(() => {
		if (selectedId === null) return;
		void tick().then(() =>
			document.getElementById(`hotspot-${selectedId}`)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
		);
	});

	function mine(hotspot: Recommendation) {
		return birds.map((bird) => ({
			bird,
			...sighting(
				hotspot.last_seen?.find((seen) => seen.species_id === bird.species_id),
				today
			)
		}));
	}
</script>

<ul class="list">
	{#each shown as { hotspot, rank } (hotspot.hotspot_id)}
		{@const status = lastSeenStatus(hotspot, birds, today)}
		{@const name = splitName(hotspot.hotspot_name)}
		{@const selected = hotspot.hotspot_id === selectedId}
		<li id="hotspot-{hotspot.hotspot_id}">
			<button
				type="button"
				class="entry"
				class:selected
				aria-expanded={selected}
				onclick={() => onselect(selected ? null : hotspot.hotspot_id)}
			>
				<span class="number" aria-hidden="true">{rank}</span>
				<span class="text">
					<span class="name">{name.main}</span>
					{#if name.alternative}<span class="alternative">{name.alternative}</span>{/if}
					<span class="meta">{distanceText(hotspot)}</span>
					{#if status}
						<span class="status" class:recent={status.recent}>{status.text}</span>
					{/if}
				</span>
			</button>

			{#if selected}
				{@const shownSpecies = hotspot.top_species}
				{@const sightings = mine(hotspot)}
				<div class="details">
					<p class="note">{t('card.meta', { count: checklistCount(hotspot) })}</p>
					{#if birds.length > 0}
						<p class="note">{t(expectedKey(hotspot.score, birds.length))}</p>
					{/if}

					{#if sightings.length > 0}
						<h4>{t('card.yourBirds')}</h4>
						<ul class="rows stacked">
							{#each sightings as entry (entry.bird.species_id)}
								<li>
									<span class="label">{entry.bird.common_name}</span>
									{#if entry.url}
										<a href={entry.url} class:recent={entry.recent} target="_blank" rel="noopener noreferrer">{entry.text}</a>
									{:else}
										<span class="value">{entry.text}</span>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}

					{#if shownSpecies.length > 0}
						<h4>{t('card.likely')}</h4>
						<ul class="rows">
							{#each shownSpecies as species (species.species_id)}
								{@const tooFew = species.checklists < fewChecklists}
								<li>
									<button type="button" class="row-button" onclick={() => onpickspecies(hotspot, species.species_id)}>
										<span class="label">{species.common_name}</span>
										<span class="dots" aria-hidden="true"></span>
										<span class="value">
											{tooFew ? t('chance.tooLittle') : t(chanceKey(species.frequency))}
										</span>
									</button>
								</li>
							{/each}
						</ul>
						<p class="note">{t('card.share')}</p>
					{:else}
						<p class="note">{t('card.noChecklists')}</p>
					{/if}

					<div class="actions">
						<button type="button" class="primary" onclick={() => onsearchhere(hotspot)}>{t('card.searchHere')}</button>
						{#if httpsUrl(hotspot.verification_url)}
							<a href={httpsUrl(hotspot.verification_url)} target="_blank" rel="noopener noreferrer">eBird ↗</a>
						{/if}
					</div>
				</div>
			{/if}
		</li>
	{/each}
</ul>
{#if hidden > 0}
	<button type="button" class="more" onclick={() => (visible += step)}>
		{t('list.showMore', { count: Math.min(step, hidden) })}
	</button>
{/if}
{#if visible > initiallyShown}
	<button type="button" class="more expanded" onclick={() => (visible = initiallyShown)}>{t('list.showFewer')}</button>
{/if}

<style>
	.list {
		margin: 0;
		padding: 0 20px;
		list-style: none;
	}

	.list > li {
		border-bottom: 1px solid var(--color-line);
	}

	/* The ink rule above the first entry. */
	.list > li:first-child {
		border-top: 1px solid var(--color-text);
	}

	.entry {
		display: flex;
		gap: 14px;
		width: 100%;
		padding: 12px 0;
		border: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		text-align: left;
		cursor: pointer;
	}

	/* A large old-style figure; the open entry's is a filled orange disc. */
	.number {
		flex-shrink: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 34px;
		height: 34px;
		border-radius: 999px;
		font-size: 1.7rem;
		font-variant-numeric: oldstyle-nums;
		line-height: 1;
	}

	.selected .number {
		background: var(--color-accent-strong);
		color: var(--color-on-accent);
		font-size: 1rem;
		font-weight: 600;
		font-variant-numeric: lining-nums;
	}

	.text {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}

	.name {
		font-size: 1rem;
		font-weight: 600;
		overflow-wrap: anywhere;
	}

	.alternative {
		color: var(--color-muted);
		font-size: 0.85rem;
		font-style: italic;
	}

	.meta {
		color: var(--color-muted);
		font-size: 0.85rem;
		font-variant-numeric: tabular-nums;
	}

	.status {
		color: var(--color-muted);
		font-size: 0.85rem;
	}

	.status.recent {
		color: var(--color-accent-text);
		font-weight: 600;
	}

	.details {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin: 0 0 16px 17px;
		padding: 0 0 0 30px;
		border-left: 1px solid var(--color-text);
	}

	h4 {
		margin: 14px 0 2px;
		font-size: 1.1rem;
		font-style: italic;
		font-weight: 500;
	}

	h4:first-of-type {
		margin-top: 6px;
	}

	.rows {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.rows li {
		display: flex;
		align-items: baseline;
		gap: 6px;
		padding: 3px 0;
		font-size: 0.9rem;
	}

	.row-button {
		display: flex;
		align-items: baseline;
		gap: 6px;
		width: 100%;
		min-height: 28px;
		padding: 0;
		border: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		text-align: left;
		cursor: pointer;
	}

	.label {
		font-weight: 500;
		overflow-wrap: anywhere;
	}

	.rows.stacked li {
		flex-direction: column;
		align-items: flex-start;
		gap: 0;
		padding: 5px 0;
	}

	.rows.stacked .value,
	.rows.stacked a {
		text-align: left;
	}

	.dots {
		flex: 1;
		min-width: 12px;
		border-bottom: 1px dotted var(--color-muted);
	}

	.value,
	.rows a {
		color: var(--color-muted);
		font-size: 0.85rem;
		font-variant-numeric: tabular-nums;
		text-align: right;
	}

	.rows a {
		text-decoration: underline;
	}

	.rows a.recent {
		color: var(--color-accent-text);
		font-weight: 600;
	}

	.note {
		margin: 0;
		color: var(--color-muted);
		font-size: 0.85rem;
		line-height: 1.45;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 16px;
		margin-top: 4px;
	}

	.primary {
		min-height: 44px;
		padding: 0 18px;
		border: 0;
		border-radius: 0;
		background: var(--color-text);
		color: var(--color-ground);
		font: inherit;
		font-weight: 600;
		cursor: pointer;
	}

	.actions a {
		color: var(--color-text);
		font-weight: 500;
		text-decoration: underline;
	}

	.more {
		margin: 6px 20px 0;
		padding: 8px 0;
		border: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 0.9rem;
		font-weight: 500;
		text-decoration: underline;
		cursor: pointer;
	}

	/* While the list is long, the button to shorten it stays at the bottom of
	   the visible sidebar. */
	.more.expanded {
		position: sticky;
		bottom: 0;
		width: calc(100% - 40px);
		border-top: 1px solid var(--color-text);
		background: var(--color-ground);
		text-align: left;
	}

	@media (max-width: 639px) {
		.more {
			min-height: 44px;
		}

		.row-button {
			min-height: 44px;
			align-items: center;
		}
	}
</style>
