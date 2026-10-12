<script lang="ts">
	import { chooseLanguage, locale, t, type Language } from '$lib/i18n.svelte';

	const languages: Language[] = ['lt', 'en'];
</script>

<div class="switch" role="group" aria-label={t('language.label')}>
	{#each languages as language (language)}
		<button
			type="button"
			class="text-button"
			lang={language}
			aria-pressed={locale.language === language}
			onclick={() => chooseLanguage(language)}
		>
			{language.toUpperCase()}
		</button>
	{/each}
</div>

<style>
	button {
		min-width: 2rem;
		font-size: 0.85rem;
	}

	@media (max-width: 639px) {
		button {
			min-width: 44px;
			min-height: 44px;
			background: var(--color-ground);
		}
	}
</style>
