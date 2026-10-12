<script lang="ts">
	import { ApiError, searchSpecies, type SpeciesMatch } from '$lib/api';
	import { t } from '$lib/i18n.svelte';

	interface Props {
		placeholder: string;
		disabled?: boolean;
		onpick: (species: SpeciesMatch) => void;
		onerror: (message: string) => void;
	}

	let { placeholder, disabled = false, onpick, onerror }: Props = $props();

	let query = $state('');
	let suggestions = $state<SpeciesMatch[]>([]);
	let timer: ReturnType<typeof setTimeout> | undefined;

	const label = (species: SpeciesMatch) => `${species.common_name} (${species.scientific_name})`;

	// Suggestions are refreshed after a pause in typing. They are a plain list
	// rather than a <datalist> because a browser hides any datalist option that
	// does not contain the typed text, and the server also matches through the
	// other language and through unaccented text.
	function oninput() {
		clearTimeout(timer);
		const typed = query.trim();
		if (typed.length < 2) {
			suggestions = [];
			return;
		}
		timer = setTimeout(async () => {
			try {
				const matches = await searchSpecies(typed);
				// A slower earlier request must not overwrite a newer one.
				if (typed === query.trim()) suggestions = matches;
			} catch (err) {
				onerror(err instanceof ApiError ? err.message : t('error.speciesSearchFailed'));
			}
		}, 200);
	}

	function onkeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			// Closing the suggestions is this key press's whole job: a dialog
			// around the box must not close with them.
			if (suggestions.length > 0) event.preventDefault();
			suggestions = [];
		} else if (event.key === 'Enter' && suggestions.length > 0) {
			event.preventDefault();
			pick(suggestions[0]);
		}
	}

	function pick(species: SpeciesMatch) {
		clearTimeout(timer);
		query = '';
		suggestions = [];
		onpick(species);
	}
</script>

<div class="search">
	<svg
		class="icon"
		width="18"
		height="18"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		aria-hidden="true"
	>
		<circle cx="11" cy="11" r="7" />
		<path d="M20 20l-4-4" />
	</svg>
	<input
		type="search"
		aria-label={placeholder}
		autocomplete="off"
		{placeholder}
		{disabled}
		bind:value={query}
		{oninput}
		{onkeydown}
	/>
	{#if suggestions.length > 0}
		<ul class="suggestions">
			{#each suggestions as suggestion (suggestion.species_id)}
				<li>
					<button type="button" onclick={() => pick(suggestion)}>{label(suggestion)}</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.search {
		position: relative;
		width: 100%;
	}

	.icon {
		position: absolute;
		top: 50%;
		left: 4px;
		transform: translateY(-50%);
		color: var(--color-muted);
		pointer-events: none;
	}

	input {
		width: 100%;
		height: 44px;
		padding: 0 4px 0 30px;
		border: 0;
		border-bottom: 1px solid var(--color-text);
		border-radius: 0;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 1rem;
	}

	input:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.suggestions {
		position: absolute;
		top: 100%;
		left: 0;
		z-index: 10;
		min-width: 100%;
		max-width: min(420px, 90vw);
		width: max-content;
		margin: 0.25rem 0 0;
		padding: 0.25rem 0;
		list-style: none;
		border: 1px solid var(--color-text);
		background: var(--color-ground);
	}

	.suggestions button {
		all: unset;
		display: block;
		box-sizing: border-box;
		width: 100%;
		padding: 0.5rem 0.75rem;
		cursor: pointer;
	}

	.suggestions button:hover,
	.suggestions button:focus-visible {
		background: var(--color-line-soft);
	}

	@media (max-width: 639px) {
		input {
			height: 48px;
			border-bottom: 0;
			background: var(--color-ground);
			box-shadow: 0 0 0 1px var(--color-text);
			padding-left: 36px;
		}

		.icon {
			left: 10px;
		}
	}
</style>
