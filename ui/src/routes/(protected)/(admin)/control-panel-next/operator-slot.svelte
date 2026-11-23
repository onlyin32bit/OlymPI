<script lang="ts">
	import { RichText } from '$lib/components/common';
	import LogPanel from '$lib/components/operator/log-panel.svelte';
	import { panel } from '$lib/operator/panel.svelte';
	import VideoControls from './video-controls.svelte';

	let question = $derived(panel.runtime?.current_question);
	let current = $derived(question && question.id === panel.controller.selected_question_id ? question : undefined);
	let heading = $derived(panel.selectedOption ? `${panel.phase.label} · ${panel.selectedOption.label}` : panel.phase.label);
	let mediaLabel = $derived(
		current?.type === 'video' ? 'Video' : current?.type === 'audio' ? 'Âm thanh' : current?.type === 'image' ? 'Hình ảnh' : ''
	);
</script>

<section class="slot cp-panel" aria-label="Khu vực riêng của người điều khiển">
	<header>
		<div class="switch" role="group" aria-label="Nội dung">
			<button type="button" aria-pressed={panel.view === 'reader'} onclick={() => (panel.view = 'reader')}>Câu hỏi</button>
			<button type="button" aria-pressed={panel.view === 'log'} onclick={() => (panel.view = 'log')}>Nhật ký</button>
		</div>
		<span class="heading" title={heading}>{heading}</span>
		{#if current}
			<span class={`visibility ${panel.controller.is_showing_question ? 'shown' : ''}`}>
				{panel.controller.is_showing_question ? 'Đang hiện' : 'Đang ẩn'}
			</span>
		{/if}
	</header>

	{#if panel.view === 'log'}
		{#if current}
			<p class="pinned"><span class="cp-label">Đáp án</span> <RichText value={current.answer || '—'} class="inline" /></p>
		{/if}
		<div class="log"><LogPanel /></div>
	{:else if current}
		<div class="reader">
			<div class="scroll">
				<div class="question"><RichText value={current.content || '—'} /></div>
				{#if mediaLabel}
					<div class="media">
						{#if current.type === 'image' && current.asset_id}
							<img src={`/api/assets/${current.asset_id}`} alt="Hình của câu hỏi" />
						{:else}
							<span class="cp-label">{mediaLabel}</span>
						{/if}
					</div>
				{/if}
				<!-- Phát media: disabled for text/image and Tăng tốc. -->
				<VideoControls />
			</div>
			<!-- Pinned so a long question never pushes the answer out of view. -->
			<div class="answer">
				<span class="cp-label">Đáp án</span>
				<RichText value={current.answer || '—'} />
			</div>
		</div>
		{#if panel.nextOption && panel.nextOption.id !== current.id}
			<footer class="next">
				<span class="cp-label">Tiếp · {panel.nextOption.label}</span>
				<div class="clamp"><RichText value={panel.nextOption.question.content || '—'} /></div>
			</footer>
		{/if}
	{:else}
		<div class="empty">
			{#if panel.nextOption}
				<span class="cp-label">Tiếp · {panel.nextOption.label}</span>
				<div class="question dim"><RichText value={panel.nextOption.question.content || '—'} /></div>
			{:else}
				<span class="cp-label">Chưa chọn câu hỏi</span>
			{/if}
		</div>
	{/if}
</section>

<style>
	.slot {
		display: flex;
		flex: 1;
		flex-direction: column;
		overflow: hidden;
	}
	header {
		display: flex;
		flex: none;
		align-items: center;
		gap: 12px;
		height: 44px;
		padding: 0 8px;
		border-bottom: 1px solid var(--cp-line);
	}
	.switch {
		display: flex;
		padding: 2px;
		border-radius: 6px;
		background: var(--cp-raised);
	}
	.switch button {
		height: 30px;
		padding: 0 12px;
		border-radius: 4px;
		color: var(--cp-muted);
		font: 500 13px/1 var(--cp-font);
		cursor: pointer;
	}
	.switch button[aria-pressed='true'] {
		background: var(--cp-surface);
		color: var(--cp-text);
		font-weight: 600;
		box-shadow: 0 0 0 1px var(--cp-line);
	}
	.heading {
		flex: 1;
		overflow: hidden;
		color: var(--cp-muted);
		font-size: 13px;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.visibility {
		flex: none;
		padding: 3px 8px;
		border-radius: 999px;
		background: var(--cp-raised);
		color: var(--cp-muted);
		font-size: 12px;
		font-weight: 600;
	}
	.visibility.shown {
		background: var(--cp-live-bg);
		color: var(--cp-live-ink);
	}
	.reader {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 10px;
		min-height: 0;
		padding: 12px 14px;
	}
	.scroll {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 10px;
		min-height: 0;
		overflow-y: auto;
	}
	.question {
		font-size: 18px;
		line-height: 1.5;
	}
	.question.dim {
		color: var(--cp-muted);
	}
	.media {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.media img {
		max-width: 100%;
		max-height: 140px;
		object-fit: contain;
		align-self: flex-start;
		border-radius: 6px;
		background: #000;
	}
	.answer {
		flex: none;
		max-height: 45%;
		overflow-y: auto;
		padding: 10px 12px;
		border-left: 3px solid var(--cp-ok);
		border-radius: 0 6px 6px 0;
		background: var(--cp-ok-bg);
		color: var(--cp-text);
		font-size: 24px;
		font-weight: 600;
		line-height: 1.3;
	}
	.answer .cp-label {
		display: block;
		margin-bottom: 2px;
		font-weight: 600;
	}
	.next {
		flex: none;
		padding: 8px 14px;
		border-top: 1px solid var(--cp-line);
		color: var(--cp-muted);
	}
	.next .clamp {
		display: -webkit-box;
		overflow: hidden;
		font-size: 13px;
		line-height: 1.4;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}
	.empty {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 14px;
	}
	.pinned {
		display: flex;
		flex: none;
		align-items: baseline;
		gap: 8px;
		padding: 8px 12px;
		border-bottom: 1px solid var(--cp-line);
		background: var(--cp-ok-bg);
		font-size: 16px;
		font-weight: 600;
	}
	.pinned :global(.inline) {
		display: inline;
	}
	.log {
		flex: 1;
		min-height: 0;
	}
</style>
