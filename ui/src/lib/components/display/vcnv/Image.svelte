<script lang="ts">
	import { assetURL, type DisplayContext } from '$lib/display/types';
	import Bells from './Bells.svelte';

	let { context }: { context: DisplayContext } = $props();
	let open = $derived(context.match?.controller.vcnv_state.image_open_state ?? []);
	let image = $derived(context.vcnv?.obstacle_asset_id ?? '');
	const tile = 'fixed h-[50cqh] w-[50cqw] border-8 border-[#e5e7eb] bg-gradient-to-tr from-[#0F247D] to-[#26164D]';

	// Each tile is cut from the picture on its own. A closed tile never has picture pixels
	// in the DOM, so dragging or selecting the board cannot expose the hidden parts.
	const tiles = [
		{ x: 0, y: 0, w: 50, h: 50, clip: 'polygon(0 0, 100% 0, 100% 55%, 55% 55%, 55% 100%, 0 100%)' },
		{ x: 50, y: 0, w: 50, h: 50, clip: 'polygon(0 0, 100% 0, 100% 100%, 45% 100%, 45% 55%, 0 55%)' },
		{ x: 0, y: 50, w: 50, h: 50, clip: 'polygon(55% 0, 55% 45%, 100% 45%, 100% 100%, 0 100%, 0 0)' },
		{ x: 50, y: 50, w: 50, h: 50, clip: 'polygon(45% 45%, 45% 0, 100% 0, 100% 100%, 0 100%, 0 45%)' },
		{ x: 27.5, y: 27.5, w: 45, h: 45, clip: 'none' }
	];

	/** Places the full picture so that the window at (x, y) shows exactly the picture's pixels there. */
	function pictureStyle(x: number, y: number): string {
		return `left: calc(50cqw - ${x}cqw); top: -${y}cqh; height: 100cqh; transform: translateX(-50%);`;
	}
</script>

<div class="h-full w-full text-[20cqh] font-bold select-none" oncontextmenu={(event) => event.preventDefault()} role="presentation">
	{#each tiles as area, i (i)}
		{#if open[i]}
			<div
				class="fixed overflow-hidden"
				style={`left: ${area.x}cqw; top: ${area.y}cqh; width: ${area.w}cqw; height: ${area.h}cqh; clip-path: ${area.clip};`}
			>
				{#if image}
					<img
						class="absolute max-w-none"
						style={pictureStyle(area.x, area.y)}
						src={assetURL(image)}
						alt=""
						draggable="false"
					/>
				{/if}
			</div>
		{/if}
	{/each}
	{#if !open[0]}
		<div class="{tile} left-0 top-0" style="clip-path: polygon(0 0, 100% 0, 100% 55%, 55% 55%, 55% 100%, 0 100%);">
			<h1 class="absolute left-[11cqw] top-[5cqh]">1</h1>
		</div>
	{/if}
	{#if !open[1]}
		<div class="{tile} right-0 top-0" style="clip-path: polygon(0 0, 100% 0, 100% 100%, 45% 100%, 45% 55%, 0 55%);">
			<h1 class="absolute right-[11cqw] top-[5cqh]">2</h1>
		</div>
	{/if}
	{#if !open[2]}
		<div class="{tile} bottom-0 left-0" style="clip-path: polygon(55% 0, 55% 45%, 100% 45%, 100% 100%, 0 100%, 0 0);">
			<h1 class="absolute bottom-[7cqh] left-[11cqw]">3</h1>
		</div>
	{/if}
	{#if !open[3]}
		<div class="{tile} bottom-0 right-0" style="clip-path: polygon(45% 45%, 45% 0, 100% 0, 100% 100%, 0 100%, 0 45%);">
			<h1 class="absolute bottom-[7cqh] right-[11cqw]">4</h1>
		</div>
	{/if}
	{#if !open[4]}
		<div
			class="fixed left-1/2 top-1/2 h-[45cqh] w-[45cqw] -translate-x-1/2 -translate-y-1/2 border-[16px] border-[#e5e7eb] bg-gradient-to-tr from-[#0F247D] to-[#26164D]"
		></div>
	{/if}
	<Bells {context} />
</div>
