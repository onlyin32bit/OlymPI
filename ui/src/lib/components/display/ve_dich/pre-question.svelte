<script lang="ts">
	import { fade, fly, scale } from 'svelte/transition';
	import type { DisplayContext } from '$lib/display/types';
	import { choiceTier, screenSlot, tierLabel } from './slot';

	// Legacy ScreenPreQuestionVD.svelte (sounds are cued by the server).
	let { context }: { context: DisplayContext } = $props();
	let slot = $derived(screenSlot(context.screen));
	let contestant = $derived(context.match?.contestants.find((value) => value.slot === slot));
	let chosen = $derived(
		(context.match?.controller.ve_dich_contestants_state[slot - 1]?.selected_question_set ?? []).map(
			choiceTier
		)
	);
	let tiers = $derived(context.site.scoring?.ve_dich?.tiers);
</script>

<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat" transition:fade|global>
	<div
		class="fixed left-0 top-0 flex h-[12cqh] w-full items-center justify-center bg-blue-100 font-number-display text-[10cqh] font-bold text-black"
	>
		{contestant?.name ?? ''}
	</div>
	<div class="fixed bottom-[26cqh] h-[7cqh] w-full bg-white" in:fly|global={{ delay: 5000, x: -1000 }}></div>
	<div class="fixed bottom-[15cqh] left-1/2 flex -translate-x-1/2 gap-[10cqw]">
		{#each ['20', '30'] as score (score)}
			<div class="flex gap-[1cqw]" in:scale|global={{ delay: score === '20' ? 5000 : 5250 }}>
				<div class="flex flex-col gap-[1cqh]">
					{#each [0, 1, 2] as item, i (item)}
						<div
							class={`flex h-[9cqh] w-[9cqh] items-center justify-center rounded-[1cqh] transition-colors duration-150 ${chosen[item] === score ? 'bg-green-400' : 'bg-white'}`}
							style={`border: 0.7cqh solid #6EB0ED; filter: drop-shadow(8px 28px 32px #335); box-shadow: inset 0.4cqh 0.4cqh 0.5cqh #777; transition-delay: ${item * 500}ms;`}
						>
							{#if chosen[item] === score}
								<svg class="h-[6.5cqh]" fill="#fff" viewBox="0 0 448 512" in:scale={{ delay: i * 500 }}
									><path
										d="M438.6 105.4c12.5 12.5 12.5 32.8 0 45.3l-256 256c-12.5 12.5-32.8 12.5-45.3 0l-128-128c-12.5-12.5-12.5-32.8 0-45.3s32.8-12.5 45.3 0L160 338.7 393.4 105.4c12.5-12.5 32.8-12.5 45.3 0z"
									/></svg
								>
							{/if}
						</div>
					{/each}
				</div>
				<div
					class="flex h-[29cqh] w-[29cqh] items-center justify-center rounded-[1cqh] bg-[#093278] text-[16cqh] font-black"
					style="border: 1cqh solid #6EB0ED; filter: drop-shadow(8px 28px 32px #335); box-shadow: inset 1cqh 1cqh 6cqh 2cqh #55a;"
				>
					{tierLabel(tiers, score)}
				</div>
			</div>
		{/each}
	</div>
</div>
