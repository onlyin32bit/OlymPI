<script lang="ts">
	import { fade } from 'svelte/transition';
	import type { DisplayContext } from '$lib/display/types';

	// Legacy ScreenPreQuestionKD.
	let { context }: { context: DisplayContext } = $props();
	let contestants = $derived([...(context.match?.contestants ?? [])].sort((a, b) => a.slot - b.slot));
</script>

<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat" transition:fade|global>
	<div class="fixed right-[2cqw] top-[20cqh] space-y-[3cqh] text-[4cqh] font-medium" in:fade|global={{ duration: 1000 }}>
		{#each contestants as contestant (contestant.slot)}
			<div
				class="flex w-[18cqw] items-center justify-between bg-gradient-to-tr from-[#093278] to-[#093278] px-[1cqw]"
				style={`border: 0.5cqh solid #6EB0ED; filter: drop-shadow(8px 8px 17px #335) brightness(${contestant.has_rung ? '1.0' : '0.4'});`}
			>
				<div>{contestant.name}</div>
			</div>
		{/each}
	</div>
</div>
