<script lang="ts">
	import { brand } from '$lib/brand';
	import { fly, fade, slide } from 'svelte/transition';
	import RichText from '$lib/components/common/rich-text.svelte';
	import { reviewContestants, type DisplayContext } from '$lib/display/types';

	let { context }: { context: DisplayContext } = $props();
	// Legacy: answered contestants first, otherwise slot order.
	let contestants = $derived(reviewContestants(context.match, false));
	let status = $derived(context.match?.controller.answer_status ?? []);
	let dim = (slot: number) => (status[slot - 1] === false ? '0.65' : '1.1');
</script>

<div class="h-full w-full bg-bg-1 bg-cover bg-no-repeat text-[8cqh] text-white">
	<img class="center-element fixed h-[50cqh] opacity-25" src="/4t.png" alt="Logo {brand.appName}" in:fade|global />
	<div
		class="fixed left-1/2 top-1/2 h-[110cqh] w-[2cqw] -translate-x-1/2 -translate-y-1/2 bg-white"
		in:fly|global={{ y: -1300, delay: 200, duration: 900 }}
	></div>
	{#each contestants as { slot, name, answer }, index (slot)}
		{#if index % 2 === 0}
			<div class="fixed" style={`left: 52cqw; top: ${20 * (index + 0.5)}cqh; filter: brightness(${dim(slot)});`}>
				<div
					class="flex min-w-[40cqw] flex-col border-[0.7cqh] border-[#e5e7eb] bg-gradient-to-tr from-[#0F247D] to-[#26164D]"
					in:fly|global={{ x: -100, y: -50, delay: index * 200, duration: 500 }}
				>
					<h1 class="text-[5cqh] font-black text-black">
						<div class="w-[25cqw] bg-white px-[2cqw]" style="clip-path: polygon(100% 0, 100% 65%, 92% 100%, 0 100%, 0 0);">
							{name}
						</div>
					</h1>
					<div
						class="answer flex h-[9cqh] min-w-[40cqw] items-center px-[2cqw] text-[7cqh] font-bold uppercase tracking-tighter"
						in:slide|global={{ delay: 1500, duration: 600 }}
					>
						<RichText value={answer ?? ''} />
					</div>
				</div>
			</div>
		{:else}
			<div
				class="fixed"
				style={`right: 52cqw; top: ${20 * (index + 0.5)}cqh; filter: brightness(${dim(slot)}) drop-shadow(8px 28px 32px #335);`}
			>
				<div
					class="flex min-w-[40cqw] flex-col items-end border-[0.7cqh] border-[#e5e7eb] bg-gradient-to-tr from-[#0F247D] to-[#26164D]"
					in:fly|global={{ x: 100, y: -50, delay: index * 180, duration: 500 }}
				>
					<h1 class="text-right text-[5cqh] font-black text-black">
						<div class="w-[25cqw] bg-white px-[2cqw]" style="clip-path: polygon(100% 0, 100% 100%, 8% 100%, 0 65%, 0 0);">
							{name}
						</div>
					</h1>
					<div
						class="answer flex h-[9cqh] min-w-[40cqw] items-center justify-end px-[2cqw] text-[7cqh] font-bold uppercase tracking-tighter"
						in:slide|global={{ delay: 1500, duration: 600 }}
					>
						<RichText value={answer ?? ''} />
					</div>
				</div>
			</div>
		{/if}
	{/each}
</div>

<style>
	.answer :global(.katex) {
		text-transform: none;
	}
</style>
