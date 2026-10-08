<script lang="ts">
	import type { Recommendation, SpeciesMatch } from '$lib/api';
	import { t } from '$lib/i18n.svelte';
	import { chanceKey, checklistCount, distanceText, fewChecklists, sighting } from '$lib/format';

	interface Props {
		hotspot: Recommendation;
		birds: SpeciesMatch[];
		onclose: () => void;
		onpickspecies: (speciesId: number) => void;
		onsearchhere: () => void;
	}

	let { hotspot, birds, onclose, onpickspecies, onsearchhere }: Props = $props();

	const today = new Date();
	const checklists = $derived(checklistCount(hotspot));
	const shown = $derived(hotspot.top_species.slice(0, 5));
	const mine = $derived(
		birds.map((bird) => ({
			bird,
			...sighting(
				hotspot.last_seen?.find((seen) => seen.species_id === bird.species_id),
				today
			)
		}))
	);
</script>

<section class="card" aria-label={hotspot.hotspot_name}>
	<div class="head">
		<div>
			<h2>{hotspot.hotspot_name}</h2>
			<p class="meta">
				{t('card.meta', { count: checklists })} · {distanceText(hotspot)}
			</p>
		</div>
		<button type="button" class="close" onclick={onclose} aria-label={t('common.close')}>✕</button>
	</div>

	{#if mine.length > 0}
		<h3>{t('card.yourBirds')}</h3>
		<ul class="mine">
			{#each mine as entry (entry.bird.species_id)}
				<li>
					<span class="dot" class:recent={entry.recent}></span>
					<span class="bird">{entry.bird.common_name}</span>
					{#if entry.url}
						<a href={entry.url} class:recent={entry.recent} target="_blank" rel="noopener noreferrer">{entry.text}</a>
					{:else}
						<span class="when">{entry.text}</span>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}

	{#if shown.length > 0}
		<h3>{t('card.likely')}</h3>
		<ul class="seen">
			{#each shown as species (species.species_id)}
				{@const percent = Math.round(species.frequency * 100)}
				{@const tooFew = species.checklists < fewChecklists}
				<li>
					<button type="button" onclick={() => onpickspecies(species.species_id)}>
						<span class="name">{species.common_name}</span>
						<span class="track"><span class="fill" style="width: {tooFew ? 0 : percent}%"></span></span>
						<span class="chance" class:thin={tooFew}>
							{t(tooFew ? 'chance.tooLittle' : chanceKey(species.frequency))}
							{#if tooFew}<small>{t('list.count', { count: species.checklists })}</small>{/if}
						</span>
					</button>
				</li>
			{/each}
		</ul>
		<p class="note">
			{t('card.share')}
		</p>
	{:else}
		<p class="note">{t('card.noChecklists')}</p>
	{/if}

	<div class="actions">
		<button type="button" class="primary" onclick={onsearchhere}>{t('card.searchHere')}</button>
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
		/* Leaves the bottom of the map to the year scrubber. */
		max-height: calc(100% - 32px - 190px);
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

	.mine {
		display: flex;
		flex-direction: column;
		gap: 8px;
		margin: 0;
		padding: 10px 12px;
		border-radius: 10px;
		background: var(--color-selected);
		list-style: none;
	}

	.mine li {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.9rem;
	}

	.bird {
		flex: 1;
		font-weight: 600;
	}

	.when,
	.mine a {
		color: var(--color-muted);
		font-size: 0.8rem;
	}

	.mine a.recent {
		color: var(--color-accent-text);
		font-weight: 600;
	}

	.dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: 999px;
		background: var(--color-control-border);
	}

	.dot.recent {
		background: var(--color-accent-strong);
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

	.chance {
		width: 6.5rem;
		font-size: 0.8rem;
		font-weight: 600;
		line-height: 1.2;
		text-align: right;
	}

	.chance small {
		display: block;
		font-size: 0.75rem;
	}

	.chance.thin {
		color: var(--color-muted);
		font-weight: 400;
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

		.close {
			width: 44px;
			height: 44px;
		}

		.seen button {
			min-height: 44px;
		}
	}
</style>
