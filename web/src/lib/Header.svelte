<script lang="ts">
	import type { SpeciesMatch } from '$lib/api';
	import { t } from '$lib/i18n.svelte';
	import InfoButton from '$lib/InfoButton.svelte';
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
		<img src="/logo.png" width="40" height="40" alt="" />
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
	<InfoButton />
</header>

<style>
	.header {
		display: flex;
		align-items: center;
		gap: 24px;
		height: 64px;
		padding: 0 20px;
		border-bottom: 1px solid var(--color-text);
		background: var(--color-ground);
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 12px;
		flex: 0 1 360px;
		min-width: 0;
	}

	.brand img {
		flex-shrink: 0;
	}

	h1 {
		margin: 0;
		overflow: hidden;
		font-size: 1.2rem;
		font-weight: 600;
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
