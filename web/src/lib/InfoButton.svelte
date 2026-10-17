<script lang="ts">
	import { t } from '$lib/i18n.svelte';

	let dialog: HTMLDialogElement;
</script>

<button type="button" class="open" aria-label={t('about.label')} onclick={() => dialog.showModal()}>
	<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
		<circle cx="12" cy="12" r="10" />
		<path d="M12 16v-5M12 8h.01" />
	</svg>
</button>

<dialog bind:this={dialog} aria-label={t('about.label')} onclick={(event) => event.target === dialog && dialog.close()}>
	<p>{t('about.text')}</p>
	<p><a href="https://ebird.org" target="_blank" rel="noopener noreferrer">{t('about.link')}</a></p>
	<button type="button" class="close" onclick={() => dialog.close()}>{t('common.close')}</button>
</dialog>

<style>
	.open {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		width: 44px;
		height: 44px;
		border: 1px solid var(--color-text);
		background: var(--color-ground);
		color: var(--color-text);
		cursor: pointer;
	}

	dialog {
		width: min(420px, calc(100% - 32px));
		padding: 1.5rem;
		border: 1px solid var(--color-text);
		background: var(--color-panel);
		color: var(--color-text);
	}

	dialog::backdrop {
		background: var(--color-backdrop);
	}

	p {
		margin: 0 0 1rem;
	}

	.close {
		min-height: 44px;
		padding: 0 20px;
		border: 0;
		background: var(--color-inverse);
		color: var(--color-on-inverse);
		font: inherit;
		cursor: pointer;
	}
</style>
