<script lang="ts">
	import { api } from '$lib/api';
	import { RichText } from '$lib/components/common';
	import { reviewContestants } from '$lib/display/types';
	import { fitText } from '$lib/display/fit-text';
	import { ruleText } from '$lib/display/rule-text';
	import type { MatchRuntimeState } from '$lib/socket/opsocket';
	import { questionView, stealWindow, turnOwner, type MainView } from '$lib/station/view';
	import Star from '@lucide/svelte/icons/star';
	import VcnvBoard from './vcnv-board.svelte';

	let { runtime, slot, view }: { runtime: MatchRuntimeState | null; slot: number; view: MainView } = $props();

	let question = $derived(questionView(runtime));
	let tiers = $derived(api.settings.current?.scoring?.ve_dich?.tiers ?? [20, 30]);
	let standings = $derived(
		[...(runtime?.contestants ?? [])].sort((a, b) =>
			view.kind === 'standings' && view.title === 'Bảng điểm' ? b.score - a.score || a.slot - b.slot : a.slot - b.slot
		)
	);
	let finishOwner = $derived(turnOwner(runtime));
	let finishState = $derived(runtime?.controller.ve_dich_contestants_state[finishOwner - 1]);
	let finishIndex = $derived(Number(/^vd\.[0-3]\.(?:20|30)\.([0-2])$/.exec(runtime?.controller.selected_question_id ?? '')?.[1] ?? -1));
</script>

{#snippet questionBlock()}
	{#if question}
		<div class="question" class:with-image={Boolean(question.imageID)}>
			<!-- Long questions shrink to fit instead of scrolling: contestants never scroll. -->
			<div class="content" use:fitText={46}><RichText value={question.content} /></div>
			{#if question.imageID}
				<img src={`/api/assets/${question.imageID}`} alt="Hình ảnh câu hỏi" />
			{/if}
		</div>
	{:else}
		<div class="waiting"></div>
	{/if}
{/snippet}

<div class="area">
	{#if view.kind === 'status'}
		<div class="center">
			<h1>{view.title}</h1>
			{#if view.detail}<p class="detail">{view.detail}</p>{/if}
		</div>
	{:else if view.kind === 'standings'}
		<h1 class="title">{view.title}</h1>
		<ol class="standings">
			{#each standings as entry (entry.slot)}
				<li class:self={entry.slot === slot}>
					<span class="name">{entry.name || `Thí sinh ${entry.slot}`}</span>
					{#if entry.class}<span class="class">{entry.class}</span>{/if}
					<strong>{entry.score}</strong>
				</li>
			{/each}
		</ol>
	{:else if view.kind === 'round'}
		{#if view.rule}
			<h1 class="title">{view.title} · Luật chơi</h1>
			<div class="rule-box" use:fitText={28}>
				<p class="rule">{ruleText(runtime?.controller.phase ?? '', api.settings.current?.scoring).content.replace(/\t/g, '')}</p>
			</div>
		{:else}
			<div class="center">
				<h1 class="round">{view.title}</h1>
				{#if view.ended}<p class="detail">Kết thúc phần thi</p>{/if}
			</div>
		{/if}
	{:else if view.kind === 'question'}
		<header class="line">
			<span>{view.round}</span>
			{#if view.turn}<span>· {view.turn}</span>{/if}
			{#if view.label}<span>· {view.label}</span>{/if}
			{#if view.ownTurn}<strong class="own">Lượt của bạn</strong>{/if}
		</header>
		{@render questionBlock()}
	{:else if view.kind === 'vcnv'}
		<VcnvBoard {runtime} />
		{@render questionBlock()}
	{:else if view.kind === 'review'}
		<h1 class="title">Câu trả lời của các thí sinh</h1>
		<ol class="review">
			{#each reviewContestants(runtime, view.timed) as entry (entry.slot)}
				<li class:wrong={runtime?.controller.answer_status[entry.slot - 1] === false} class:self={entry.slot === slot}>
					<span class="name">{entry.name || `Thí sinh ${entry.slot}`}</span>
					<strong class="answer">{entry.answer || '—'}</strong>
					{#if view.timed}<span class="time">{entry.answer ? `${(entry.time_ms / 1000).toFixed(2)}s` : ''}</span>{/if}
				</li>
			{/each}
		</ol>
	{:else if view.kind === 've_dich'}
		<header class="line">
			<span>Về đích</span>
			{#if view.choosing}<span>· Chọn gói câu hỏi</span>{/if}
			{#if view.ownTurn}<strong class="own">Lượt của bạn</strong>{/if}
		</header>
		{#if finishState?.selected_question_set.length}
			<ol class="package" aria-label="Gói câu hỏi">
				{#each finishState.selected_question_set as choice, index (choice)}
					<li class:current={index === finishIndex}>
						<span>Câu {index + 1}</span>
						<strong>{tiers[choice.startsWith('30_') ? 1 : 0]}</strong>
						{#if finishState.starred_on === index + 1}<Star class="star" aria-label="Ngôi sao hy vọng" />{/if}
					</li>
				{/each}
			</ol>
		{/if}
		{#if stealWindow(runtime)}
			<p class="steal" role="status">Cướp quyền trả lời</p>
		{/if}
		{#if !view.choosing}{@render questionBlock()}{/if}
	{/if}
</div>

<style>
	.area {
		display: flex;
		flex-direction: column;
		gap: 0.8em;
		height: 100%;
		padding: 1em 1.2em;
		overflow: hidden;
	}
	.center {
		display: flex;
		flex: 1;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.4em;
		text-align: center;
	}
	h1 {
		font-size: 2em;
		font-weight: 800;
		line-height: 1.15;
	}
	.round {
		font-size: 3em;
		text-transform: uppercase;
	}
	.title {
		font-size: 1.4em;
	}
	.detail {
		color: var(--muted);
		font-size: 1.1em;
	}
	.line {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.4em;
		color: var(--muted);
		font-weight: 600;
	}
	.own {
		margin-left: auto;
		padding: 0.2em 0.7em;
		border-radius: 999px;
		background: var(--text);
		color: #fff;
		font-size: 1.1em;
		text-transform: uppercase;
	}
	.question {
		display: grid;
		flex: 1;
		grid-template-rows: minmax(0, 1fr);
		gap: 1em;
		min-height: 0;
	}
	.question.with-image {
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
	}
	.content {
		min-height: 0;
		overflow: hidden;
		font-weight: 600;
		line-height: 1.4;
	}
	.question img {
		width: 100%;
		max-height: 100%;
		object-fit: contain;
		align-self: start;
		border-radius: 8px;
		background: var(--raised);
	}
	.waiting {
		flex: 1;
	}
	.rule-box {
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}
	.rule {
		line-height: 1.55;
		white-space: pre-line;
	}
	.standings,
	.review {
		display: grid;
		gap: 0.5em;
	}
	.standings li,
	.review li {
		display: flex;
		align-items: baseline;
		gap: 0.8em;
		padding: 0.6em 0.9em;
		border-radius: 8px;
		background: var(--raised);
	}
	.standings li.self,
	.review li.self {
		box-shadow: inset 4px 0 0 var(--text);
	}
	.standings .name,
	.review .name {
		font-weight: 700;
		text-transform: uppercase;
	}
	.standings .class {
		color: var(--muted);
		font-size: 0.85em;
	}
	.standings strong {
		margin-left: auto;
		font-size: 1.5em;
	}
	.review .answer {
		flex: 1;
		overflow: hidden;
		font-size: 1.3em;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.review .time {
		font-weight: 700;
	}
	.review li.wrong {
		color: var(--faint);
	}
	.review li.wrong .answer {
		text-decoration: line-through;
	}
	.package {
		display: flex;
		gap: 0.6em;
	}
	.package li {
		display: flex;
		align-items: center;
		gap: 0.5em;
		padding: 0.4em 0.9em;
		border: 2px solid var(--line);
		border-radius: 8px;
	}
	.package li span {
		color: var(--muted);
	}
	.package li strong {
		font-size: 1.3em;
	}
	.package li.current {
		border-color: var(--text);
	}
	.package :global(.star) {
		width: 1.1em;
		height: 1.1em;
		color: #ca8a04;
		fill: #facc15;
	}
	.steal {
		padding: 0.4em 0.8em;
		border-radius: 8px;
		background: #fee2e2;
		color: #b91c1c;
		font-size: 1.6em;
		font-weight: 800;
		text-align: center;
		text-transform: uppercase;
	}
</style>
