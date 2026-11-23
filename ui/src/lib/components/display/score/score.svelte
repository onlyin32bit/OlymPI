<script lang="ts">
	import { brand } from '$lib/brand';
	import type { DisplayContext } from '$lib/display/types';
	import { scale } from 'svelte/transition';

	let { context }: { context: DisplayContext } = $props();
	let contestants = $derived(
		[...(context.match?.contestants ?? [])].sort((a, b) => a.slot - b.slot)
	);
	let stage = $derived(context.match?.stage?.trim() ?? '');
</script>

<!-- Legacy Scores.svelte: scores change in place (no number animation). -->
<div class="h-full w-full bg-bg-2 bg-cover bg-no-repeat text-[6cqh] text-white">
	<img class="fixed top-[3cqh] right-[9cqw] h-[10cqh]" src="/4t.png" alt="Logo {brand.appName}" />
	<img class="fixed top-[3cqh] right-[2cqw] h-[10.5cqh]" src="/logo-school.png" alt="Logo {brand.schoolName}" />
	<h1
		class="fixed top-[8cqh] w-full text-center font-sans text-[16cqh] font-bold text-blue-50"
		style="text-shadow: 0.1cqh 0.5cqh 1cqh rgba(103, 103, 140, 1);"
	>
		BẢNG ĐIỂM
	</h1>
	{#if stage}
		<h2
			class="fixed top-[27cqh] w-full text-center font-sans text-[8cqh] font-bold text-blue-50 uppercase opacity-80"
			style="text-shadow: 0.1cqh 0.5cqh 1cqh rgba(103, 103, 140, 1);"
		>
			{stage}
		</h2>
	{/if}
	<div class="fixed top-[42cqh] left-1/2 flex -translate-x-1/2 gap-[2.5cqw] font-sans">
		{#each contestants as contestant, i (contestant.slot)}
			<div
				class="w-[22.5cqw] border-[0.8cqh] border-slate-100 text-[4.3cqh]"
				style="filter: drop-shadow(8px 28px 32px #335);"
				in:scale|global={{ delay: i * 100 }}
			>
				<div
					class="flex h-[8cqh] items-center justify-center bg-gradient-to-l from-[#00bfff] via-[#00fdff] to-[#00bfff] font-black text-[#2632b1]"
				>
					{contestant.name.toUpperCase()}
				</div>
				<div
					class="flex h-[25cqh] items-center justify-center text-[15cqh] font-black"
					style="background: radial-gradient(circle, rgba(18,32,177,1) 0%, rgba(31,31,83,1) 100%);"
				>
					<div>{contestant.score}</div>
				</div>
			</div>
		{/each}
	</div>
</div>
