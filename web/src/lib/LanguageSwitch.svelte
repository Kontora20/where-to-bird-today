<script lang="ts">
	import { chooseLanguage, locale, t, type Language } from '$lib/i18n.svelte';

	const languages: Language[] = ['lt', 'en'];
</script>

<div class="group" role="group" aria-label={t('language.label')}>
	{#each languages as language (language)}
		<button
			type="button"
			lang={language}
			aria-pressed={locale.language === language}
			onclick={() => chooseLanguage(language)}
		>
			{language.toUpperCase()}
		</button>
	{/each}
</div>

<style>
	.group {
		display: flex;
		gap: 2px;
		padding: 2px;
		border-radius: 8px;
		background: var(--color-control-track);
	}

	button {
		min-width: 2.25rem;
		padding: 0.3rem 0.5rem;
		border: none;
		border-radius: 6px;
		background: none;
		color: var(--color-text);
		font: inherit;
		font-size: 0.85rem;
		cursor: pointer;
	}

	button[aria-pressed='true'] {
		background: var(--color-inverse);
		color: var(--color-on-inverse);
	}

	@media (max-width: 639px) {
		button {
			min-width: 44px;
			min-height: 44px;
		}
	}
</style>
