<script lang="ts">
	import { fade, scale, slide } from 'svelte/transition';
	import RichText from '$lib/components/common/rich-text.svelte';
	import QuestionMedia from '../common/question-media.svelte';
	import { serverClock } from '$lib/display/clock.svelte';
	import { timerElapsed, type DisplayContext } from '$lib/display/types';
	import Bells from './Bells.svelte';

	let { context }: { context: DisplayContext } = $props();
	const clock = serverClock();
	let question = $derived(context.match?.current_question);
	let displayQuestion = $derived(Boolean(context.match?.controller.is_showing_question));
	let questionNumber = $derived.by(() => {
		const id = question?.id ?? '';
		if (id.endsWith('.center')) return 5;
		const row = /\.rows\.(\d+)$/.exec(id);
		return row ? Number(row[1]) + 1 : 1;
	});
	let image = $derived(question?.type === 'image' && question.asset_id ? question.asset_id : '');

	// Legacy tweened(0→15): bar shows while running/paused or after running out; an early stop clears it.
	let timer = $derived(context.match?.timer);
	let percent = $derived.by(() => {
		if (!timer || timer.kind === 'obstacle' || timer.duration_ms <= 0) return 0;
		const elapsed = timerElapsed(timer, clock.now);
		if (!timer.is_running && !timer.is_paused && elapsed < timer.duration_ms) return 0;
		return (elapsed / timer.duration_ms) * 100;
	});

	// Legacy sizing: start 3vh, grow 0.2vh while the text is under 60% of the box. Run as a
	// synchronous measure loop; additionally shrink only if the text would leave the frame.
	let container: HTMLDivElement | undefined = $state();
	let text: HTMLDivElement | undefined = $state();
	let fontSize = $state(3);
	function fit() {
		if (!container || !text) return;
		const frame = container.offsetParent instanceof HTMLElement ? container.offsetParent.clientHeight : 0;
		const box = container.clientHeight;
		if (box <= 0) return;
		let size = 3;
		text.style.fontSize = `${size}cqh`;
		while (size < 60 && (text.offsetHeight / box) * 100 < 60) {
			size += 0.2;
			text.style.fontSize = `${size}cqh`;
		}
		while (frame > 0 && size > 1 && text.offsetHeight > frame * 0.94) {
			size -= 0.2;
			text.style.fontSize = `${size}cqh`;
		}
		fontSize = size;
	}
	$effect(() => {
		void question?.id;
		void question?.content;
		void image;
		fit();
		const fonts = document.fonts;
		let alive = true;
		void fonts?.ready.then(() => alive && fit());
		return () => (alive = false);
	});
</script>

<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat">
	<img
		class="absolute left-[45cqw] top-[2cqh] h-[90cqh] w-[90cqw] -translate-x-1/2"
		style="filter: drop-shadow(8px 28px 32px #335);"
		src="/bg-ques.svg"
		alt="BG"
		in:scale|global={{ duration: 750 }}
	/>
	<div
		class="question-number absolute left-[6.7cqw] top-[11.4cqh] flex h-[10cqh] w-[8cqw] items-center justify-center bg-white font-number-display text-[9cqh] font-extrabold text-black"
		class:invisible={questionNumber > 5}
		style="clip-path: polygon(100% 0, 100% 70%, 80% 100%, 0 100%, 0 0);"
		in:slide|global={{ delay: 750, duration: 600 }}
	>
		{questionNumber == 5 ? 'TT' : questionNumber}
	</div>
	<div class="absolute left-[5cqw] top-[17cqh] h-[60cqh] w-[70cqw] font-semibold" bind:this={container}>
		<div
			class="absolute left-[40cqw] top-1/2 z-50 w-[70cqw] -translate-x-1/2 -translate-y-1/2 text-left"
			style={`font-size: ${fontSize}cqh;`}
			bind:this={text}
		>
			<div class="whitespace-pre-line" class:invisible={!displayQuestion}>
				<RichText class="font-question" value={question?.content ?? ''} />
				{#if image}
					<div class="relative mt-[2cqh] h-[30cqh] w-full">
						<QuestionMedia {context} assetID={image} type="image" visible={displayQuestion} />
					</div>
				{/if}
			</div>
		</div>
	</div>
	<div in:fade|global={{ delay: 800 }}>
		<div
			class="progress-cap absolute bottom-[13.6cqh] right-[86cqw] h-[5cqh] w-[2.92cqw] bg-gradient-to-b from-slate-200 to-sky-200"
			style="clip-path: polygon(0 0, 100% 0, 100% 100%, 93% 100%);"
		></div>
		<div
			class="progress-track absolute bottom-[13.6cqh] right-[15.9cqw] flex h-[5cqh] w-[70.2cqw] items-center bg-gradient-to-br from-slate-200 via-sky-200 to-slate-300"
		>
			{#if percent > 0}
				<div
					class="progress-fill h-1/2 w-full rounded-[2cqh] bg-red-500"
					style={`box-shadow: 0 0 1cqh 0.1cqh rgba(256,0,0,0.5); width: ${percent}%;`}
					in:scale|global={{ delay: 200 }}
				></div>
				<div
					class="progress-dot animate-shadow-pulse size-[3.5cqh] -translate-x-[1.5cqh] rounded-full bg-red-600"
					in:scale|global={{ duration: 200, opacity: 1 }}
				></div>
			{/if}
		</div>
	</div>
	<Bells {context} shadow={false} />
</div>
