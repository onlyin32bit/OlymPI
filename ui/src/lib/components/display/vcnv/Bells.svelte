<script lang="ts">
	import { fly } from 'svelte/transition';
	import type { DisplayContext } from '$lib/display/types';

	let { context, shadow = true }: { context: DisplayContext; shadow?: boolean } = $props();
	let contestants = $derived(context.match?.contestants ?? []);
</script>

<div class="fixed bottom-[2.5cqh] left-[2.5cqw] flex gap-[2cqw] text-[5cqh] font-bold">
	{#each contestants as contestant (contestant.slot)}
		{#if contestant.has_rung}
			<div
				class="flex items-center justify-center gap-[1cqw] rounded-[1cqh] border-[0.7cqh] border-[#e5e7eb] bg-gradient-to-tr from-[#0F247D] to-[#26164D] px-[1cqw]"
				style={shadow ? 'filter: drop-shadow(8px 28px 32px #335);' : ''}
				in:fly|global={{ x: 100 }}
			>
				<svg class="h-[5cqh] animate-legacy-wiggle" fill="#fff" viewBox="0 0 448 512" aria-hidden="true">
					<path
						d="M224 0c-17.7 0-32 14.3-32 32l0 19.2C119 66 64 130.6 64 208l0 18.8c0 47-17.3 92.4-48.5 127.6l-7.4 8.3c-8.4 9.4-10.4 22.9-5.3 34.4S19.4 416 32 416l384 0c12.6 0 24-7.4 29.2-18.9s3.1-25-5.3-34.4l-7.4-8.3C401.3 319.2 384 273.9 384 226.8l0-18.8c0-77.4-55-142-128-156.8L256 32c0-17.7-14.3-32-32-32zm45.3 493.3c12-12 18.7-28.3 18.7-45.3l-64 0-64 0c0 17 6.7 33.3 18.7 45.3s28.3 18.7 45.3 18.7s33.3-6.7 45.3-18.7z"
					/>
				</svg>
				{contestant.name}
			</div>
		{/if}
	{/each}
</div>
