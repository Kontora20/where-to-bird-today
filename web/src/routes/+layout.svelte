<script lang="ts">
	import '../app.css';
	import { locale, t } from '$lib/i18n.svelte';

	let { children } = $props();

	$effect(() => {
		document.documentElement.lang = locale.language;
	});
</script>

<svelte:head>
	<link rel="icon" type="image/png" href="/favicon.png" />
</svelte:head>

<svelte:boundary>
	{@render children()}
	{#snippet failed(_error, reset)}
		<div class="crashed" role="alert">
			<p>{t('error.crashed')}</p>
			<button type="button" onclick={reset}>{t('common.retry')}</button>
		</div>
	{/snippet}
</svelte:boundary>

<style>
	.crashed {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		padding: 48px 16px;
	}

	.crashed button {
		min-height: 44px;
		padding: 0 20px;
		border: 0;
		border-radius: 8px;
		background: var(--color-inverse);
		color: var(--color-on-inverse);
		font: inherit;
		cursor: pointer;
	}
</style>
