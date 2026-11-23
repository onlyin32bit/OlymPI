<script lang="ts">
	import { onMount } from 'svelte';
	import { Title3D, type TitlePalette } from '$lib/display/title-3d';

	let {
		text,
		vcnv = false,
		palette = 'silver',
		duration = 1400,
		centered = false,
		still = false,
		shadow = '0.4cqh 0.7cqh 0.9cqh rgb(15 25 60 / 0.32)'
	}: {
		text: string;
		vcnv?: boolean;
		palette?: TitlePalette;
		duration?: number;
		/** Vertical centre of the frame instead of the lower half. */
		centered?: boolean;
		/** Final pose only, no entrance animation. */
		still?: boolean;
		/** CSS drop-shadow parameters for the title layer. */
		shadow?: string;
	} = $props();

	let canvas = $state<HTMLCanvasElement>();
	let title: Title3D | undefined;

	/** Preview only: freezes the entrance at this many milliseconds. */
	export function seek(ms: number): void {
		title?.seek(ms);
	}
	export function length(): number {
		return title?.length ?? 0;
	}

	onMount(() => {
		if (!canvas) return;
		title = new Title3D(canvas, {
			text,
			palette,
			fontPx: centered ? (vcnv ? 130 : 190) : vcnv ? 108 : 150,
			centerY: centered ? 540 : 190,
			duration,
			totalMs: 3000,
			still
		});
		void title.init();
		return () => title?.dispose();
	});
</script>

<!-- 1920×540 WebGL layer over the lower half of the canvas; the legacy 16:9 frame scales it. -->
{#if centered}
	<canvas bind:this={canvas} width="1920" height="1080" class="fixed top-0 left-0 h-full w-full" style={`filter: drop-shadow(${shadow});`} aria-label={text}></canvas>
{:else}
	<canvas bind:this={canvas} width="1920" height="540" class="fixed top-[50cqh] left-0 h-[50cqh] w-full" style={`filter: drop-shadow(${shadow});`} aria-label={text}></canvas>
{/if}
