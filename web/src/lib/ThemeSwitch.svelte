<script lang="ts">
	import { t } from '$lib/i18n.svelte';
	import { chooseTheme, theme, type ThemeChoice } from '$lib/theme.svelte';

	const choices: ThemeChoice[] = ['light', 'dark'];

	const next = () => choices[(choices.indexOf(theme.choice) + 1) % choices.length];
</script>

{#snippet icon(choice: ThemeChoice)}
	<svg
		viewBox="0 0 24 24"
		width="18"
		height="18"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		{#if choice === 'light'}
			<circle cx="12" cy="12" r="4" />
			<path
				d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"
			/>
		{:else}
			<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
		{/if}
	</svg>
{/snippet}

<div class="switch group" role="group" aria-label={t('theme.label')}>
	{#each choices as choice (choice)}
		<button
			type="button"
			class="text-button"
			aria-pressed={theme.choice === choice}
			onclick={() => chooseTheme(choice)}
		>
			{t(`theme.${choice}`)}
		</button>
	{/each}
</div>

<button
	type="button"
	class="cycle"
	aria-label={t('theme.change', { theme: t(`theme.${theme.choice}`) })}
	onclick={() => chooseTheme(next())}
>
	{@render icon(theme.choice)}
</button>

<style>
	.group button {
		font-size: 0.85rem;
		text-transform: capitalize;
	}

	.cycle {
		display: none;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		border: 1px solid var(--color-text);
		background: var(--color-ground);
		color: var(--color-text);
		cursor: pointer;
	}

	@media (max-width: 639px) {
		.group {
			display: none;
		}

		.cycle {
			display: flex;
		}
	}
</style>
