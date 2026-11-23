<script lang="ts">
	import { brand } from '$lib/brand';
	import { untrack } from 'svelte';
	import { componentFor, phaseFor, screenFor } from '$lib/display/phase-manifest';
	import { branchKey, type DisplayContext } from '$lib/display/types';

	let { context, loading = false }: { context: DisplayContext; loading?: boolean } = $props();

	let shown = $derived.by<DisplayContext>(() => {
		const phase = phaseFor(context.phase);
		const screen = screenFor(phase.id, context.screen);
		return { ...context, phase: phase.id, screen: screen.id };
	});
	let key = $derived(branchKey(shown.phase, shown.screen));
	let Scene = $derived(componentFor(shown.phase, shown.screen));

	let resetBoundary: (() => void) | null = null;
	let failedContext: DisplayContext | null = null;

	function onSceneError(error: unknown, reset: () => void): void {
		console.error('Display scene failed', error);
		failedContext = untrack(() => context);
		resetBoundary = reset;
	}

	// A crashed scene shows the legacy error page and retries on the next context change.
	$effect(() => {
		const next = context;
		untrack(() => {
			if (!resetBoundary || next === failedContext) return;
			const reset = resetBoundary;
			resetBoundary = null;
			failedContext = null;
			reset();
		});
	});
</script>

{#if loading}
	<!-- Legacy Screen.svelte loader. -->
	<div class="legacy-page flex h-full flex-col items-center justify-center gap-8">
		<img src="/4t.png" alt="{brand.appName} logo" class="h-[150px]" />
		<span class="legacy-loading legacy-loading-ring"></span>
	</div>
{:else}
	<svelte:boundary onerror={onSceneError}>
		{#key key}
			<Scene context={shown} />
		{/key}

		{#snippet failed()}
			<!-- Legacy routes/display/+error.svelte. -->
			<div class="legacy-page flex h-full flex-col items-center justify-center gap-8">
				<img src="/4t.png" alt="{brand.appName} logo" class="h-[150px]" />
				<span class="legacy-loading legacy-loading-dots"></span>
			</div>
		{/snippet}
	</svelte:boundary>
{/if}

<style>
	/* daisyUI 4 `loading loading-lg` (+ ring/dots), light theme base colours. */
	.legacy-page {
		background-color: oklch(100% 0 0);
		color: oklch(27.8078% 0.029596 256.847952);
	}
	.legacy-loading {
		pointer-events: none;
		display: inline-block;
		aspect-ratio: 1 / 1;
		width: 2.5rem;
		background-color: currentColor;
		mask-size: 100%;
		mask-repeat: no-repeat;
		mask-position: center;
	}
	.legacy-loading-ring {
		mask-image: url("data:image/svg+xml,%3Csvg width='44' height='44' viewBox='0 0 44 44' xmlns='http://www.w3.org/2000/svg' stroke='%23fff'%3E%3Cg fill='none' fill-rule='evenodd' stroke-width='2'%3E%3Ccircle cx='22' cy='22' r='1'%3E%3Canimate attributeName='r' begin='0s' dur='1.8s' values='1; 20' calcMode='spline' keyTimes='0; 1' keySplines='0.165, 0.84, 0.44, 1' repeatCount='indefinite' /%3E%3Canimate attributeName='stroke-opacity' begin='0s' dur='1.8s' values='1; 0' calcMode='spline' keyTimes='0; 1' keySplines='0.3, 0.61, 0.355, 1' repeatCount='indefinite' /%3E%3C/circle%3E%3Ccircle cx='22' cy='22' r='1'%3E%3Canimate attributeName='r' begin='-0.9s' dur='1.8s' values='1; 20' calcMode='spline' keyTimes='0; 1' keySplines='0.165, 0.84, 0.44, 1' repeatCount='indefinite' /%3E%3Canimate attributeName='stroke-opacity' begin='-0.9s' dur='1.8s' values='1; 0' calcMode='spline' keyTimes='0; 1' keySplines='0.3, 0.61, 0.355, 1' repeatCount='indefinite' /%3E%3C/circle%3E%3C/g%3E%3C/svg%3E");
	}
	.legacy-loading-dots {
		mask-image: url("data:image/svg+xml,%3Csvg width='24' height='24' viewBox='0 0 24 24' xmlns='http://www.w3.org/2000/svg'%3E%3Cstyle%3E.spinner_qM83%7Banimation:spinner_8HQG 1.05s infinite%7D.spinner_oXPr%7Banimation-delay:.1s%7D.spinner_ZTLf%7Banimation-delay:.2s%7D@keyframes spinner_8HQG%7B0%25,57.14%25%7Banimation-timing-function:cubic-bezier(0.33,.66,.66,1);transform:translate(0)%7D28.57%25%7Banimation-timing-function:cubic-bezier(0.33,0,.66,.33);transform:translateY(-6px)%7D100%25%7Btransform:translate(0)%7D%7D%3C/style%3E%3Ccircle class='spinner_qM83' cx='4' cy='12' r='3'/%3E%3Ccircle class='spinner_qM83 spinner_oXPr' cx='12' cy='12' r='3'/%3E%3Ccircle class='spinner_qM83 spinner_ZTLf' cx='20' cy='12' r='3'/%3E%3C/svg%3E");
	}
</style>
