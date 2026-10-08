<script lang="ts">
	import type { SpeciesMatch } from '$lib/api';
	import { t } from '$lib/i18n.svelte';
	import LanguageSwitch from '$lib/LanguageSwitch.svelte';
	import SpeciesSearch from '$lib/SpeciesSearch.svelte';
	import ThemeSwitch from '$lib/ThemeSwitch.svelte';

	interface Props {
		searchPlaceholder: string;
		searchDisabled: boolean;
		onpickspecies: (species: SpeciesMatch) => void;
		onerror: (message: string) => void;
	}

	let { searchPlaceholder, searchDisabled, onpickspecies, onerror }: Props = $props();
</script>

<header class="header">
	<div class="brand">
		<svg width="40" height="37" viewBox="0 0 70 64" aria-hidden="true">
			<path
				d="M8 34 C6 16 24 6 42 10 C60 14 68 30 60 46 C52 60 30 62 16 54 C10 50 9 42 8 34Z"
				fill="var(--color-inverse)"
			/>
			<path
				d="M40 12 C52 10 64 22 58 34 C52 42 40 38 36 28 C34 20 34 14 40 12Z"
				fill="var(--color-accent)"
			/>
		</svg>
		<h1>{t('app.title')}</h1>
	</div>
	<div class="search">
		<SpeciesSearch
			placeholder={searchPlaceholder}
			disabled={searchDisabled}
			onpick={onpickspecies}
			{onerror}
		/>
	</div>
	<LanguageSwitch />
	<ThemeSwitch />
</header>

<style>
	.header {
		display: flex;
		align-items: center;
		gap: 24px;
		height: 64px;
		padding: 0 20px;
		border-bottom: 1px solid var(--color-line);
		background: var(--color-ground);
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 12px;
		flex: 0 1 360px;
		min-width: 0;
	}

	.brand svg {
		flex-shrink: 0;
	}

	h1 {
		margin: 0;
		overflow: hidden;
		font-size: 1.2rem;
		letter-spacing: -0.01em;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.search {
		display: flex;
		flex: 1;
		justify-content: center;
		min-width: 160px;
		max-width: 520px;
		margin: 0 auto;
	}

	@media (max-width: 639px) {
		.header {
			position: fixed;
			top: 12px;
			right: 12px;
			left: 12px;
			z-index: 30;
			gap: 8px;
			height: auto;
			padding: 0;
			border: 0;
			background: none;
		}

		.brand {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
		}

		.search {
			min-width: 0;
			max-width: none;
			margin: 0;
		}
	}
</style>
