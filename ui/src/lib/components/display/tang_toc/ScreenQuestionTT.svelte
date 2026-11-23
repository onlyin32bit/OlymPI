<script lang="ts">
	import { scale, slide } from 'svelte/transition';
	import RichText from '$lib/components/common/rich-text.svelte';
	import QuestionMedia from '../common/question-media.svelte';
	import { serverClock } from '$lib/display/clock.svelte';
	import { timerProgress, type DisplayContext } from '$lib/display/types';
	import { legacyTextSize } from './legacy-text-size';

	let { context }: { context: DisplayContext } = $props();
	const clock = serverClock();
	let runtime = $derived(context.match);
	let question = $derived(runtime?.current_question);
	let timer = $derived(runtime?.timer);
	let displayQuestion = $derived(Boolean(runtime?.controller.is_showing_question && question));
	let percent = $derived(timerProgress(timer, clock.now) * 100);
	// Legacy `$time > 0`: the video appears once the question timer has started and then plays
	// to its end, never paused or seeked by the timer.
	let startedFor = $state('');
	$effect(() => {
		const id = question?.id;
		if (id && timer && (timer.is_running || timer.elapsed_ms > 0)) startedFor = id;
	});
	let timerStarted = $derived(Boolean(question?.id) && startedFor === question?.id);
	let content = $derived(question?.content ?? '');
</script>

<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat" in:slide|global={{ duration: 1500 }}>
	<div>
		<img
			class="absolute left-[10cqw] w-[73cqw]"
			style="filter: drop-shadow(8px 28px 32px #335);"
			src="/bg-tt.svg"
			alt="BG"
			in:scale|global={{ delay: 900, duration: 1000 }}
		/>
		<div
			class="fixed bottom-[3.2cqh] left-[5cqw] h-[92.5cqh] w-[6cqw] overflow-hidden bg-gray-200 outline-8"
			style="filter: drop-shadow(8px 28px 32px #335); border: 1cqh solid #88badf;"
			in:scale|global={{ delay: 900, duration: 1000 }}
		>
			<div
				class="absolute top-0 h-[110%] w-full"
				style={`background: linear-gradient(to top, #f00 ${percent}%, rgba(0,0,0,0) ${percent + 10}%);`}
			></div>
		</div>
		<div class="fixed left-[14cqw] top-[7cqh] h-[12cqh] w-[64cqw] font-semibold">
			<div
				class="absolute left-1/2 top-1/2 z-50 w-[63cqw] -translate-x-1/2 -translate-y-1/2 text-center"
				style="font-size: 3cqh; text-shadow: 3px 1px 11px rgba(103, 103, 140, 1);"
				use:legacyTextSize={{ key: content, step: 0.1 }}
			>
				<div class="whitespace-pre-line" class:invisible={!displayQuestion}>
					<RichText class="font-question" value={content} />
				</div>
			</div>
		</div>
		<div class="fixed bottom-[4.42cqh] left-[14cqw] h-[72cqh] w-[65cqw]">
			{#if question?.asset_id && displayQuestion}
				<QuestionMedia
					{context}
					assetID={question.asset_id}
					type={question.type}
					play={timerStarted}
					class="absolute left-1/2 h-full -translate-x-1/2"
				/>
			{/if}
		</div>
	</div>
</div>
