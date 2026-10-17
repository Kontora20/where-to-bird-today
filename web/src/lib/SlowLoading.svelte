<script lang="ts">
	import { t } from '$lib/i18n.svelte';

	let { active }: { active: boolean } = $props();

	// Quick answers pass without a flicker: the bar only shows after a second.
	const delayMs = 1000;
	let slow = $state(false);

	$effect(() => {
		if (!active) {
			slow = false;
			return;
		}
		const timer = setTimeout(() => (slow = true), delayMs);
		return () => clearTimeout(timer);
	});
</script>

{#if slow}
	<div class="slow" role="status">
		<div class="track"><div class="bar"></div></div>
		<p>{t('common.stillLoading')}</p>
	</div>
{/if}

<style>
	.slow {
		padding: 0 20px 8px;
	}

	.track {
		height: 4px;
		overflow: hidden;
		background: var(--color-line);
	}

	.bar {
		width: 35%;
		height: 100%;
		background: var(--color-accent-strong);
		animation: slide 1.2s ease-in-out infinite;
	}

	p {
		margin: 6px 0 0;
		color: var(--color-muted);
		font-size: 0.85rem;
	}

	@keyframes slide {
		from {
			transform: translateX(-100%);
		}
		to {
			transform: translateX(290%);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.bar {
			width: 100%;
			animation: none;
		}
	}
</style>
