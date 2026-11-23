<script lang="ts">
	import { onMount, type Snippet } from 'svelte';

	let {
		fullScreen = false,
		interactive = false,
		children
	}: { fullScreen?: boolean; interactive?: boolean; children: Snippet } = $props();
	let stage = $state<HTMLDivElement>();
	let content = $state<HTMLDivElement>();

	function resize(): void {
		if (!stage || !content) return;
		const scale = Math.min(stage.clientWidth / 1920, stage.clientHeight / 1080);
		content.style.transform = `translate(${(stage.clientWidth - 1920 * scale) / 2}px, ${(stage.clientHeight - 1080 * scale) / 2}px) scale(${scale})`;
	}

	onMount(() => {
		resize();
		const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(resize);
		if (stage) observer?.observe(stage);
		if (!observer) window.addEventListener('resize', resize);
		return () => {
			observer?.disconnect();
			if (!observer) window.removeEventListener('resize', resize);
		};
	});
</script>

<div
	bind:this={stage}
	class="relative aspect-[16/9] overflow-hidden bg-black"
	style={fullScreen ? 'height: 100svh; width: 100svw;' : 'height: 100%; width: 100%;'}
>
	<!-- The 1920×1080 legacy "viewport": cqw/cqh = legacy vw/vh, fixed = this box. -->
	<div
		bind:this={content}
		class="display-canvas absolute top-0 left-0 h-[1080px] w-[1920px] origin-top-left overflow-hidden select-none"
		style:pointer-events={interactive ? 'auto' : 'none'}
	>
		{@render children()}
	</div>
</div>

<style>
	/* Legacy /display layout: bg-black text-white, font-sans 'Segoe UI'. */
	.display-canvas {
		container-type: size;
		background-color: #000;
		color: #fff;
		font-family: var(--font-display);
		line-height: 1.5;
	}
</style>
