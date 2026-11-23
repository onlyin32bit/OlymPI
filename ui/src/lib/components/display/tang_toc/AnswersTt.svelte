<script lang="ts">
	import { brand } from '$lib/brand';
	import { animate } from 'motion';
	import { onMount } from 'svelte';
	import RichText from '$lib/components/common/rich-text.svelte';
	import { reviewContestants, type DisplayContext } from '$lib/display/types';

	let { context }: { context: DisplayContext } = $props();
	// New ordering: answers by time, non-answers last.
	let contestants = $derived(reviewContestants(context.match, true));
	let dimUnanswered = $state(false);

	onMount(() => {
		const timeout = window.setTimeout(() => (dimUnanswered = true), 1900);
		return () => window.clearTimeout(timeout);
	});

	function logoAction(node: HTMLElement) {
		const animation = animate(node, { opacity: [0, 0.25], scale: [0.8, 1] }, { duration: 1.2, ease: 'easeOut' });
		return { destroy: () => animation.stop() };
	}

	function barAction(node: HTMLElement) {
		const animation = animate(node, { y: ['-1080px', '-50%'], opacity: [0, 1] }, { duration: 1.1, ease: [0.22, 1, 0.36, 1] });
		return { destroy: () => animation.stop() };
	}

	function rowAction(node: HTMLElement, index: number) {
		const animation = animate(node, { x: [500, 0], opacity: [0, 1] }, {
			delay: 0.3 + index * 0.25, type: 'spring', stiffness: 100, damping: 15
		});
		return { destroy: () => animation.stop() };
	}

	function answerAction(node: HTMLElement, index: number) {
		const animation = animate(node, { clipPath: ['inset(0 100% 0 0)', 'inset(0 0% 0 0)'] }, {
			delay: 1.5 + index * 0.25, duration: 0.6, ease: 'easeOut'
		});
		return { destroy: () => animation.stop() };
	}

	function wrong(slot: number, answer: string | undefined): boolean {
		return context.match?.controller.answer_status[slot - 1] === false || (dimUnanswered && !answer);
	}
</script>

<div class="h-full w-full overflow-hidden bg-bg-1 bg-cover bg-no-repeat text-[8cqh] text-white">
	<img use:logoAction id="logo-4t" class="center-element fixed h-[50cqh] opacity-0" src="/4t.png" alt="Logo {brand.appName}" />

	<div use:barAction id="vertical-bar" class="fixed left-[23cqw] top-1/2 h-[110cqh] w-[3cqw] bg-white opacity-0"></div>

	<div class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 space-y-[3cqh]">
		{#each contestants as contestant, index (contestant.slot)}
			<div use:rowAction={index} class="contestant-row opacity-0">
				<div
					class="flex min-w-[65cqw] flex-col border-[0.7cqh] border-[#e5e7eb] bg-gradient-to-tr from-[#0F247D] to-[#26164D]"
					style={`filter: brightness(${wrong(contestant.slot, contestant.answer) ? '0.65' : '1.1'}) drop-shadow(8px 28px 32px #335);`}
				>
					<div class="z-50 text-[5cqh] font-black text-black">
						<div class="w-[25cqw] bg-white px-[2cqw]" style="clip-path: polygon(100% 0, 100% 65%, 92% 100%, 0 100%, 0 0);">
							{contestant.name}
						</div>
					</div>
					<div
						use:answerAction={index}
						class="answer-content z-50 flex h-[9cqh] min-w-[40cqw] items-center px-[2cqw]"
						style="clip-path: inset(0 100% 0 0)"
					>
						<span class="answer mr-[2cqw] text-[7cqh] font-bold uppercase tracking-tighter">
							<RichText value={contestant.answer ?? ''} />
						</span>
						<span class="font-legacy-mono ml-auto font-medium tracking-tighter">{(contestant.time_ms / 1000).toFixed(2)}</span>
					</div>
				</div>
			</div>
		{/each}
	</div>
</div>

<style>
	.answer :global(.katex) {
		text-transform: none;
	}
</style>
