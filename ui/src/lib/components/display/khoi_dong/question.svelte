<script lang="ts">
	import { scale, slide } from 'svelte/transition';
	import { untrack } from 'svelte';
	import RichText from '$lib/components/common/rich-text.svelte';
	import { serverClock } from '$lib/display/clock.svelte';
	import { assetURL, timerElapsed, type DisplayContext } from '$lib/display/types';

	// Legacy ScreenQuestionKD. One instance serves ques_chung or all of ques_ts1..4.
	let { context }: { context: DisplayContext } = $props();
	const clock = serverClock();

	let runtime = $derived(context.match);
	let controller = $derived(runtime?.controller);
	let question = $derived(runtime?.current_question ?? null);
	let contestants = $derived([...(runtime?.contestants ?? [])].sort((a, b) => a.slot - b.slot));
	let currentContestantTurn = $derived(/^ques_ts[1-4]$/.test(context.screen) ? Number(context.screen.slice(-1)) - 1 : -1);
	let isEnd = $derived(controller?.selected_question_id === '' && Boolean(controller?.is_showing_question));
	let questionID = $derived(isEnd ? '' : (question?.id ?? ''));
	let questionNumber = $derived.by(() => {
		const index = Number(questionID.split('.').at(-1));
		return Number.isInteger(index) ? index + 1 : 1;
	});
	let questionContent = $derived(isEnd ? '' : (question?.content ?? ''));

	// Legacy blanks the text for 500 ms when the question changes inside the same screen.
	let blank = $state(false);
	let previousID = '';
	$effect(() => {
		const id = questionID;
		untrack(() => {
			const changed = previousID !== '' && id !== '' && id !== previousID;
			if (id !== '') previousID = id;
			if (!changed) return;
			blank = true;
		});
		if (!untrack(() => blank)) return;
		const timeout = setTimeout(() => (blank = false), 500);
		return () => clearTimeout(timeout);
	});
	let displayQuestion = $derived(Boolean(controller?.is_showing_question) && !blank);

	// Legacy tweened 0→3: blank until started, blank again when stopped early, '0' at the natural end.
	let timer = $derived(runtime?.timer);
	let duration = $derived((timer?.duration_ms ?? 0) / 1000);
	let time = $derived(timer ? timerElapsed(timer, clock.now) / 1000 : 0);
	let timeStatus = $derived(
		Boolean(timer && duration > 0 && !blank && (timer.is_running || timer.is_paused || time >= duration))
	);
	let turn = $derived(timeStatus ? time / duration : 0);

	// Legacy sizing: +0.2vh while the text is under 50% of the box, -0.5vh while over 80%;
	// then shrink only if it would still overflow the frame.
	let containerHeight = $state(0);
	let textBox: HTMLDivElement | undefined = $state();
	let fontSize = $state(6);
	$effect(() => {
		const node = textBox;
		const box = containerHeight;
		void questionContent;
		void isEnd;
		if (!node || box <= 0) return;
		const fit = () => {
			let size = 6;
			const apply = (value: number) => (node.style.fontSize = `${value}cqh`);
			for (let step = 0; step < 400; step++) {
				apply(size);
				const percent = (node.clientHeight / box) * 100;
				if (percent < 50) size += 0.2;
				else if (percent > 80) size -= 0.5;
				else break;
				if (size <= 0.5) break;
			}
			apply(size);
			const frame = node.parentElement!;
			while (size > 0.5 && (node.scrollWidth > node.clientWidth || node.clientHeight > frame.clientHeight)) {
				size -= 0.1;
				apply(size);
			}
			fontSize = size;
		};
		fit();
		void document.fonts?.ready.then(fit);
	});
</script>

<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat">
	<div class="relative left-[1cqw] top-[17cqh] h-[65cqh] w-[80cqw] font-semibold" bind:clientHeight={containerHeight}>
		<img
			class="absolute top-[-10cqh] w-[80cqw]"
			style="filter: drop-shadow(8px 28px 32px #335);"
			src="/bg-ques.svg"
			alt="BG"
			in:scale|global={{ duration: 3000 }}
		/>
		<div
			class="absolute left-[4cqw] top-[-1cqh] flex h-[10cqh] w-[8cqw] items-center justify-center bg-white font-number-display text-[8cqh] font-extrabold text-black"
			style="clip-path: polygon(100% 0, 100% 70%, 80% 100%, 0 100%, 0 0);"
			in:slide|global={{ delay: 4500, duration: 1200 }}
			class:invisible={!questionID}
		>
			{questionNumber}
		</div>
		{#if question?.type === 'image' && !isEnd}
			<div class:invisible={!displayQuestion}>
				<div class="absolute left-1/2 w-[55cqw] -translate-x-[45%] whitespace-pre-line text-[4.2cqh]">
					<RichText class="font-question" value={questionContent} />
				</div>
				{#if question.asset_id}
					<img class="absolute left-1/2 top-[15cqh] h-[50cqh] -translate-x-1/2" src={assetURL(question.asset_id)} alt="" />
				{/if}
			</div>
		{:else}
			<div
				bind:this={textBox}
				class="absolute left-1/2 top-1/2 z-50 w-[65cqw] -translate-x-1/2 -translate-y-1/2 text-left"
				class:text-center={!questionContent}
				style={`font-size: ${fontSize}cqh;`}
			>
				<div class="whitespace-pre-line" class:invisible={!displayQuestion}>
					{#if questionContent}<RichText class="font-question" value={questionContent} />{:else}KẾT THÚC{/if}
				</div>
			</div>
		{/if}
	</div>

	<div class="fixed right-[2cqw] top-[20cqh] space-y-[3cqh] text-[4cqh] font-medium">
		{#each contestants as contestant (contestant.slot)}
			<div
				class="flex w-[18cqw] items-center justify-between bg-gradient-to-tr from-[#093278] to-[#093278] px-[1cqw]"
				style={`border: 0.5cqh solid #6EB0ED; filter: drop-shadow(8px 8px 17px #335) brightness(${contestant.has_rung || contestant.slot - 1 === currentContestantTurn ? '1.0' : '0.4'});`}
			>
				<div>{contestant.name}</div>
			</div>
		{/each}
	</div>
	<div
		class="fixed bottom-[12.5cqh] right-[2cqw] h-[21.5cqh] w-[18cqw] rounded-[1cqh] bg-cover bg-no-repeat backdrop-blur"
		style={`border: 0.7cqh solid #6EB0ED; filter: drop-shadow(8px 28px 32px #335); background: conic-gradient(red, ${turn}turn, rgba(256,256,256,0.2), ${turn + 0.05}turn, rgba(256,256,256,0.2));`}
		in:scale|global={{ duration: 3000 }}
	>
		<div
			class="center-element absolute flex h-[18cqh] w-[16cqw] items-center justify-center rounded-[1cqh] bg-gradient-to-tr from-[#093278] to-[#093278] text-[13cqh] font-bold"
		>
			{timeStatus ? (duration - time).toFixed(0) : ''}
		</div>
	</div>
</div>
