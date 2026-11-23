<script lang="ts">
	import { assetURL, type DisplayContext } from '$lib/display/types';

	/**
	 * Legacy question media: image → plain <img>; video → muted autoplay rendered only while
	 * `play` is true and left to run to its end (never paused or seeked by the timer); audio →
	 * nothing (Phát media plays it through the sound output).
	 */
	let {
		assetID,
		type,
		play = false,
		visible = true,
		class: className = 'asset',
		context: _context // eslint-disable-line @typescript-eslint/no-unused-vars
	}: {
		assetID: string;
		type: string;
		play?: boolean;
		visible?: boolean;
		class?: string;
		context?: DisplayContext;
	} = $props();
	let url = $derived(assetURL(assetID));
</script>

{#if assetID && visible}
	{#key assetID}
		{#if type === 'image'}
			<img class={className} src={url} alt="" />
		{:else if type === 'video' && play}
			<video class={className} src={url} autoplay muted playsinline preload="auto"></video>
		{/if}
	{/key}
{/if}

<style>
	.asset {
		position: absolute;
		left: 50%;
		height: 100%;
		max-width: 100%;
		object-fit: contain;
		transform: translateX(-50%);
	}
</style>
