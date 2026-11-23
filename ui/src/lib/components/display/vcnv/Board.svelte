<script lang="ts">
	import { fade, scale } from 'svelte/transition';
	import { serverClock } from '$lib/display/clock.svelte';
	import { timerElapsed, type DisplayContext } from '$lib/display/types';
	import Bells from './Bells.svelte';

	let { context }: { context: DisplayContext } = $props();
	const clock = serverClock();
	let vcnvState = $derived(context.match?.controller.vcnv_state);
	let rowStates = $derived(vcnvState?.row_states ?? []);
	let rowLengths = $derived(context.vcnv?.row_lengths ?? []);
	let rowAnswers = $derived(
		(context.vcnv?.row_answers ?? []).map((answer) =>
			Array.from((answer ?? '').replaceAll(/[\s-]/g, '').toUpperCase())
		)
	);
	// Legacy bar: tweened 0→15 s; only the obstacle timer drives it (running or frozen).
	let timer = $derived(context.match?.timer);
	let percent = $derived.by(() => {
		if (!timer || timer.kind !== 'obstacle' || timer.duration_ms <= 0) return 0;
		return (timerElapsed(timer, clock.now) / timer.duration_ms) * 100;
	});

	function rowColor(state: string | undefined): string {
		if (state === 's' || state === 'c') return 'a0a0bf';
		if (state === 'w') return 'aaa';
		return '4040af';
	}
</script>

<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat">
	<div class="relative -left-[5cqw] -top-[3cqh] h-full w-full">
		<div in:fade|global>
			<img
				class="absolute left-1/2 top-1/2 h-[95cqh] w-[90cqw] -translate-x-1/2 -translate-y-1/2"
				style="filter: drop-shadow(8px 28px 32px #335);"
				src="/bg-vcnv.svg"
				alt=""
			/>
		</div>
		{#if vcnvState?.has_started}
			<div
				class="absolute left-2/3 top-[10cqh] w-[77cqw] -translate-x-3/4 bg-white text-center text-[7.7cqh] font-bold leading-normal text-black"
				style="clip-path: polygon(98% 0, 100% 50%, 98% 99%, 2% 100%, 0 53%, 2% 0);"
				in:scale|global={{ duration: 900 }}
			>
				{#if vcnvState.has_solved}
					<h1 class="text-red-500" in:scale|global>{(context.vcnv?.obstacle ?? '').toUpperCase()}</h1>
				{:else}
					<h1>CHƯỚNG NGẠI VẬT CÓ {context.vcnv?.obstacle_length ?? 0} CHỮ CÁI</h1>
				{/if}
			</div>
			<div
				class="absolute left-1/2 top-2/3 flex -translate-x-[55%] -translate-y-3/4 flex-col gap-[4cqh]"
				in:fade|global={{ duration: 1000 }}
			>
				{#each rowLengths as length, i (i)}
					<div class="relative">
						<div class="flex justify-center gap-[0.5cqh]">
							<div
								class="absolute right-[-7cqw] flex size-[10cqh] items-center justify-center rounded-[2.5cqh] border-[7px] border-[#083b7b] bg-gradient-to-t from-[#0a357c] to-[#395693] text-[5cqh] font-bold"
							>
								{i + 1}
							</div>
							{#each Array.from({ length: Math.max(0, length) }, (_, j) => j) as j (j)}
								<div
									class="font-question flex size-[9.3cqh] items-center justify-center rounded-full border-[0.7cqh] border-[#e5e7eb] text-[6cqh] font-black text-blue-950"
									style={`background: radial-gradient(circle at 45% 40%, #bbcfe8, #${rowColor(rowStates[i])}); box-shadow: 0.6cqh 0.65cqh 1px rgba(100, 100, 100, 0.6), inset 0.5cqh 0.5cqh 5px rgba(0, 0, 0, 0.6); filter: brightness(${rowStates[i] === 'w' ? '0.65' : '1.1'});`}
								>
									{rowStates[i] === 'c' ? (rowAnswers[i]?.[j] ?? '') : ''}
								</div>
							{/each}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
	<div
		class="fixed bottom-[14.4cqh] right-[3.5cqw] h-[75cqh] w-[6cqw] overflow-hidden bg-white"
		style="filter: drop-shadow(8px 28px 32px #335); border: 1cqh solid #88badf;"
		in:fade|global
	>
		<div
			class="absolute top-0 w-full"
			style={`height: 110%; background: linear-gradient(to top, #f00 ${percent}%, rgba(0,0,0,0) ${percent + 10}%);`}
		></div>
	</div>
	<Bells {context} />
</div>
