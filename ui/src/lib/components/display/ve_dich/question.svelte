<script lang="ts">
	import { fade, scale, slide } from 'svelte/transition';
	import { serverClock } from '$lib/display/clock.svelte';
	import { timerElapsed, type DisplayContext } from '$lib/display/types';
	import RichText from '$lib/components/common/rich-text.svelte';
	import QuestionMedia from '../common/question-media.svelte';
	import VeDichVideo from './ve-dich-video.svelte';
	import { legacyTextSize } from './legacy-text-size';
	import { screenSlot, tierLabel } from './slot';

	// Legacy ScreenQuestionVD.svelte (sounds are cued by the server).
	let { context }: { context: DisplayContext } = $props();
	const clock = serverClock();
	const timePreset: Record<string, number> = { '20': 15, '30': 20 };
	const STEP_MS = 833;
	const RINGER_MS = 1500;

	let runtime = $derived(context.match);
	let controller = $derived(runtime?.controller);
	let question = $derived(runtime?.current_question);
	let slot = $derived(screenSlot(context.screen));
	let contestants = $derived([...(runtime?.contestants ?? [])].sort((a, b) => a.slot - b.slot));
	let contestant = $derived(contestants.find((value) => value.slot === slot));
	let others = $derived(contestants.filter((value) => value.slot !== slot));
	let displayQuestion = $derived(Boolean(controller?.is_showing_question));

	// Question id: vd.<slot-1>.<tier>.<number-1>.
	let idParts = $derived((controller?.selected_question_id ?? '').split('.'));
	let isOwnQuestion = $derived(idParts[0] === 'vd' && idParts.length === 4);
	let tier = $derived(isOwnQuestion ? idParts[2] : '');
	let questionNumber = $derived(isOwnQuestion ? Number(idParts[3]) + 1 : 0);
	let preset = $derived(timePreset[tier] ?? 0);
	let starStatus = $derived(
		questionNumber > 0 &&
			controller?.ve_dich_contestants_state[slot - 1]?.starred_on === questionNumber
	);
	let content = $derived(
		controller?.selected_question_id === '' && displayQuestion ? 'KẾT THÚC' : (question?.content ?? '')
	);

	let timer = $derived(runtime?.timer);
	let isSteal = $derived(timer?.kind === 'steal');
	let timerStarted = $derived(
		Boolean(timer && timer.duration_ms > 0 && (timer.is_running || timer.is_paused || timer.elapsed_ms > 0))
	);

	// Legacy tween: once the question timer starts it runs 0 → preset; a reset or the steal
	// timer never rewinds it, a new question does.
	let started = $state<{ id: string; at: number } | null>(null);
	$effect(() => {
		const id = controller?.selected_question_id ?? '';
		if (started && started.id !== id) started = null;
		if (timerStarted && !isSteal && timer?.kind !== 'obstacle' && id) {
			const at = clock.now - timerElapsed(timer, clock.now);
			if (!started || Math.abs(started.at - at) > 250) started = { id, at };
		}
	});
	let time = $derived.by(() => {
		if (!preset) return 0;
		if (timerStarted && !isSteal) return Math.min(preset, timerElapsed(timer, clock.now) / 1000);
		if (started) return Math.min(preset, Math.max(0, (clock.now - started.at) / 1000));
		return 0;
	});
	let timeStatus = $derived(Boolean(started) || (timerStarted && !isSteal));
	let ring = $derived(preset ? time / preset : 0);

	// Steal (legacy timer === 5): highlight the other three in turn, two passes, 833 ms each;
	// once someone rings highlight the ringer for 1.5 s, then nobody.
	let ringer = $derived(contestants.find((value) => value.has_rung));
	let ringSeen = $state<{ steal: number; slot: number; at: number } | null>(null);
	$effect(() => {
		if (!isSteal || !timer || !ringer) return;
		if (ringSeen?.steal === timer.started_at) return;
		ringSeen = { steal: timer.started_at, slot: ringer.slot, at: clock.now };
	});
	let highlight = $derived.by(() => {
		if (!isSteal || !timer) return -1;
		if (ringSeen && ringSeen.steal === timer.started_at) {
			return clock.now - ringSeen.at < RINGER_MS ? ringSeen.slot : -1;
		}
		const step = Math.floor(timerElapsed(timer, clock.now) / STEP_MS);
		return step < others.length * 2 ? (others[step % others.length]?.slot ?? -1) : -1;
	});

	let container = $state<HTMLElement>();
	let isImage = $derived(question?.type === 'image' && Boolean(question.asset_id));
</script>

<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat">
	<!-- Legacy bind:clientHeight (Svelte 4) made this container position: relative. -->
	<div class="relative left-[1cqw] top-[17cqh] h-[65cqh] w-[80cqw]" bind:this={container}>
		<img
			class="absolute top-[-10cqh] w-[80cqw]"
			style="filter: drop-shadow(8px 28px 32px #335);"
			src="/bg-ques.svg"
			alt="BG"
			in:scale|global={{ duration: 3000 }}
		/>
		<div
			class="absolute left-[3.9cqw] top-[-1.2cqh] flex h-[8cqh] items-center justify-center bg-white px-[2cqw] font-number-display text-[6.5cqh] font-extrabold text-black"
			style="clip-path: polygon(100% 0, 100% 65%, 94% 100%, 0 100%, 0 0);"
			in:slide|global={{ delay: 3000, duration: 1200 }}
		>
			{contestant?.name ?? ''}
		</div>
		<div
			class="absolute left-1/2 top-1/2 z-50 w-[65cqw] -translate-x-1/2 -translate-y-1/2 text-left font-semibold"
			class:text-center={!content}
			use:legacyTextSize={{ container, key: `${controller?.selected_question_id}:${content}:${isImage}` }}
		>
			<div class="whitespace-pre-line" class:invisible={!displayQuestion}>
				<RichText class="font-question" value={content} />
				{#if isImage && question?.asset_id}
					<div class="relative mt-[2cqh] h-[40cqh] w-full">
						<QuestionMedia {context} assetID={question.asset_id} type="image" visible={displayQuestion} />
					</div>
				{/if}
			</div>
		</div>
	</div>

	<div class="fixed right-[2cqw] top-[26cqh] space-y-[3cqh] text-[4cqh] font-semibold" in:fade|global={{ duration: 1000 }}>
		{#each others as other (other.slot)}
			<div
				style={`filter: drop-shadow(8px 8px 17px #335); ${highlight === other.slot ? 'outline: 0.7cqh solid #fff; box-shadow: 0 0 1cqh 1cqh #fff' : ''}`}
			>
				<div
					class="flex w-[18cqw] items-center justify-between bg-[#093278] px-[1cqw]"
					style={`border: 0.6cqh solid #6EB0ED; filter: brightness(${other.has_rung ? '1.0' : '0.3'});`}
				>
					<div>{other.name}</div>
				</div>
			</div>
		{/each}
	</div>

	<div class="fixed bottom-[12.5cqh] right-[2cqw] space-y-[2cqh]" in:scale|global={{ duration: 3000 }}>
		<div
			class="flex h-[8cqh] w-[18cqw] items-center justify-center rounded-[1cqh] bg-[#093278] text-[4.5cqh] font-bold transition-colors duration-500"
			style={`border: 0.7cqh solid #${starStatus ? 'ff5' : '6EB0ED'}; filter: drop-shadow(8px 28px 32px #335);`}
		>
			{`Câu ${tierLabel(context.site.scoring?.ve_dich?.tiers, tier)} điểm`}
		</div>
		<div
			class="h-[21.5cqh] w-[18cqw] rounded-[1cqh] bg-cover bg-no-repeat backdrop-blur"
			style={`border: 0.7cqh solid #6EB0ED; filter: drop-shadow(8px 28px 32px #335); background: conic-gradient(red, ${ring}turn, rgba(256,256,256,0.2), ${ring + 0.05}turn, rgba(256,256,256,0.2));`}
		>
			<div
				class="center-element absolute flex h-[18cqh] w-[16cqw] items-center justify-center rounded-[1cqh] bg-[#093278] text-[14cqh] font-bold"
			>
				{timeStatus ? (preset - time).toFixed(0) : displayQuestion && preset ? preset : ''}
			</div>
		</div>
	</div>
	{#if starStatus}
		<img class="fixed right-[18cqw] top-[-5cqh] h-[47cqh]" src="/star.gif" alt="Star" in:scale />
	{/if}
</div>
<VeDichVideo {context} {question} />
