<script lang="ts">
	import { brand } from '$lib/brand';
	import type { DisplayContext } from '$lib/display/types';
	import { fade, fly, scale } from 'svelte/transition';

	let { context }: { context: DisplayContext } = $props();
	let contestants = $derived(
		[...(context.match?.contestants ?? [])].sort((a, b) => a.slot - b.slot)
	);
	// Legacy title: dictionary(game) + game_number, i.e. the match stage only.
	let stage = $derived(context.match?.stage?.trim() ?? '');
</script>

<!-- Legacy Main.svelte. -->
<div
	class="main-board pointer-events-none flex h-full w-full bg-bg-2 bg-cover bg-no-repeat text-[8cqh] font-semibold tracking-wide text-white select-none"
>
	<img class="fixed top-[5cqh] left-[3cqw] h-[16cqh]" src="/4t.png" alt="Logo {brand.appName}" in:fade|global />
	<img
		class="school-logo fixed top-[5.5cqh] right-[3cqw] h-[17cqh]"
		src="/logo-school.png"
		alt="Logo {brand.schoolName}"
		in:fade|global
	/>
	{#if stage}
		<h1
			class="fixed top-[1.5cqh] left-0 w-full text-center font-sans text-[17cqh] font-bold text-blue-50 uppercase"
			style="text-shadow: 0.1cqh 0.5cqh 1cqh rgba(103, 103, 140, 1);"
			in:scale|global={{ duration: 700 }}
		>
			{stage}
		</h1>
	{/if}
	<div class="fixed top-[28cqh] left-1/2 flex w-[80cqw] -translate-x-1/2 flex-col gap-[5.5cqh]">
		{#each contestants as contestant, index (contestant.slot)}
			<div
				class="flex items-center gap-[4cqw] overflow-hidden rounded-[1cqh]"
				style="filter: drop-shadow(0.5cqh 0.5cqh 1cqh #336); background: radial-gradient(circle, #033076 0%, #00244e 100%);"
				in:fly|global={{ x: -200, delay: index * 200 }}
			>
				<div
					class="contestant-name w-[43cqw] bg-gradient-to-l from-[#00bfff] via-[#00fdff] to-[#00bfff] px-[3cqw] font-number-display font-black text-[#003]"
					style="clip-path: polygon(100% 0, 100% 65%, 92% 100%, 0 100%, 0 0);"
				>
					{contestant.name}
				</div>
				<div class="font-header-text text-[6.7cqh] text-[#00fdff]">
					{contestant.class}
				</div>
			</div>
		{/each}
	</div>
</div>
