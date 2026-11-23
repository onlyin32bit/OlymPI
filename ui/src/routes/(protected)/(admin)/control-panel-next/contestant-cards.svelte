<script lang="ts">
	import * as Popover from '$lib/components/ui/popover';
	import { panel } from '$lib/operator/panel.svelte';
	import { veDichSlot } from '$lib/operator/questions';
	import { SLOTS, batchPoints, questionInfo, verdictAllowed, type Slot } from '$lib/operator/scoring';
	import BellOff from '@lucide/svelte/icons/bell-off';
	import BellRing from '@lucide/svelte/icons/bell-ring';
	import Delete from '@lucide/svelte/icons/delete';
	import Eraser from '@lucide/svelte/icons/eraser';
	import Lock from '@lucide/svelte/icons/lock';
	import LockOpen from '@lucide/svelte/icons/lock-open';
	import Minus from '@lucide/svelte/icons/minus';
	import Plus from '@lucide/svelte/icons/plus';
	import Undo2 from '@lucide/svelte/icons/undo-2';

	let drafts = $derived(panel.drafts);
	let liveKey = $derived(drafts.liveKey);
	let info = $derived(questionInfo(liveKey));
	let owner = $derived.by(() => {
		if (info?.owner) return info.owner;
		if (panel.controller.phase === 've_dich') return veDichSlot(panel.runtime);
		return Number(/^ques_ts([1-4])$/.exec(panel.controller.screen)?.[1] ?? 0);
	});
	let pendingLabels = $derived(
		drafts.state.batches
			.filter((batch) => Object.values(batchPoints(batch, drafts.rules)).some((value) => value !== 0))
			.map((batch) => batch.label)
	);
	let hasVerdicts = $derived(SLOTS.some((slot) => drafts.verdict(liveKey, slot) !== undefined));
	let canClear = $derived(
		Boolean(panel.runtime && panel.runtime.status !== 'completed' && panel.runtime.status !== 'archived')
	);

	function contestant(slot: Slot) {
		return panel.runtime?.contestants.find((entry) => entry.slot === slot);
	}

	function canJudge(slot: Slot): boolean {
		if (!info || !panel.runtime || panel.busy) return false;
		if (!verdictAllowed({ kind: info.kind, owner: info.owner }, slot)) return false;
		// Về đích: only the owner, or a contestant who won the steal bell, may be judged.
		if (info.kind === 've_dich' && slot !== info.owner) return Boolean(contestant(slot)?.has_rung);
		return true;
	}

	function format(value: number): string {
		return value > 0 ? `+${value}` : value < 0 ? String(value) : '±0';
	}

	function seconds(ms: number): string {
		return ms > 0 ? `${(ms / 1000).toFixed(2)}s` : '';
	}

	function keypad(slot: Slot, key: number | 'sign' | 'back' | 'clear'): void {
		if (panel.busy) return;
		const current = drafts.pending[slot];
		const negative = current < 0;
		const absolute = Math.abs(current);
		let next = current;
		if (key === 'sign') next = -current;
		else if (key === 'back') next = (negative ? -1 : 1) * Math.floor(absolute / 10);
		else if (key === 'clear') next = 0;
		else next = (negative ? -1 : 1) * Math.min(1000, absolute * 10 + key);
		drafts.setTotal(slot, next);
	}
</script>

<div class="board">
	<div class="cards">
		{#each SLOTS as slot (slot)}
			{@const entry = contestant(slot)}
			{@const verdict = drafts.verdict(liveKey, slot)}
			{@const pending = drafts.pending[slot]}
			{@const ring = panel.ringOrder.indexOf(slot)}
			{@const judge = canJudge(slot)}
			<article
				class="card"
				class:turn={owner === slot}
				class:rung={ring >= 0}
				aria-label={entry?.name || `Thí sinh ${slot}`}
			>
				<header>
					<span class="slot">TS{slot}</span>
					{#if owner === slot}<span class="tag live">Đang thi</span>{/if}
					{#if ring >= 0}<span class="tag armed"><BellRing />{ring + 1}</span>{/if}
					{#if info?.kind === 've_dich' && slot !== info.owner && entry?.has_rung}<span class="tag armed">Cướp</span>{/if}
					<button
						type="button"
						class="lock"
						class:on={entry?.locked}
						aria-pressed={entry?.locked ?? false}
						aria-label={entry?.locked ? `Mở khóa thí sinh ${slot}` : `Khóa thí sinh ${slot}`}
						title={entry?.locked ? 'Mở khóa' : 'Khóa'}
						disabled={!entry || panel.busy}
						onclick={() => panel.setLock(slot, !entry?.locked)}
					>
						{#if entry?.locked}<Lock />{:else}<LockOpen />{/if}
					</button>
				</header>
				<p class="name" title={entry?.name}>{entry?.name || '—'}</p>
				<div class="score-row">
					<strong class="score cp-num">{entry?.score ?? 0}</strong>
					<Popover.Root>
						<Popover.Trigger
							class={`pending cp-num ${pending !== 0 ? 'has' : ''}`}
							aria-label={`Điểm chưa ghi của thí sinh ${slot}: ${format(pending)}`}
							disabled={!entry || panel.busy}>{format(pending)}</Popover.Trigger
						>
						<Popover.Content class="w-56" align="center">
							<p class="mb-2 text-sm font-medium">TS{slot} · chưa ghi <span class="cp-num">{format(pending)}</span></p>
							<div class="grid grid-cols-3 gap-1.5">
								{#each [1, 2, 3, 4, 5, 6, 7, 8, 9] as digit (digit)}
									<button type="button" class="cp-btn" onclick={() => keypad(slot, digit)}>{digit}</button>
								{/each}
								<button type="button" class="cp-btn" onclick={() => keypad(slot, 'sign')}>+/−</button>
								<button type="button" class="cp-btn" onclick={() => keypad(slot, 0)}>0</button>
								<button type="button" class="cp-btn" aria-label="Xóa một chữ số" onclick={() => keypad(slot, 'back')}><Delete /></button>
								<button type="button" class="cp-btn col-span-3" onclick={() => keypad(slot, 'clear')}>Đặt về ±0</button>
							</div>
						</Popover.Content>
					</Popover.Root>
					<div class="nudge">
						<button type="button" class="cp-btn" aria-label={`Trừ ${drafts.rules.adjust.down} điểm thí sinh ${slot}`} disabled={!entry || panel.busy} onclick={() => drafts.nudge(slot, -1)}><Minus /></button>
						<button type="button" class="cp-btn" aria-label={`Cộng ${drafts.rules.adjust.up} điểm thí sinh ${slot}`} disabled={!entry || panel.busy} onclick={() => drafts.nudge(slot, 1)}><Plus /></button>
					</div>
				</div>
				<p class="answer" title={entry?.answer}>
					<span class="text">{entry?.answer || '—'}</span>
					<span class="time cp-num">{seconds(entry?.time_ms ?? 0)}</span>
				</p>
				<div class="verdicts">
					<button
						type="button"
						class="cp-btn is-ok"
						aria-pressed={verdict === 'correct'}
						aria-keyshortcuts={String(slot)}
						disabled={!judge}
						onclick={() => panel.verdict(slot, 'correct')}>Đúng</button
					>
					<button
						type="button"
						class="cp-btn is-bad"
						aria-pressed={verdict === 'wrong'}
						aria-keyshortcuts={`Shift+${slot}`}
						disabled={!judge}
						onclick={() => panel.verdict(slot, 'wrong')}>Sai</button
					>
				</div>
			</article>
		{/each}
	</div>
	<footer>
		<button
			type="button"
			class="cp-btn"
			disabled={!hasVerdicts || drafts.saving || panel.busy}
			onclick={() => panel.clearVerdicts()}>Xoá Đúng/Sai</button
		>
		<p class="summary" title={pendingLabels.join(', ')}>
			{#if drafts.hasPending}<span class="dot"></span>Chưa ghi · {pendingLabels.join(', ')}
			{:else if drafts.state.lastSave}Đã ghi · {drafts.state.lastSave.reason}{/if}
		</p>
		{#if drafts.hasPending}
			<button type="button" class="cp-btn is-ghost" disabled={drafts.saving || panel.busy} onclick={() => drafts.discard()}>Bỏ</button>
		{/if}
		<button
			type="button"
			class="cp-btn"
			disabled={!canClear || drafts.saving || panel.busy}
			onclick={() => panel.clearAnswers()}><Eraser />Xoá đáp án</button
		>
		<button
			type="button"
			class="cp-btn"
			disabled={!canClear || drafts.saving || panel.busy}
			onclick={() => panel.clearBells()}><BellOff />Xoá chuông</button
		>
		<button
			type="button"
			class="cp-btn"
			disabled={!drafts.state.lastSave || drafts.saving || panel.busy}
			onclick={() => panel.undo()}><Undo2 />Hoàn tác</button
		>
		<button
			type="button"
			class="cp-btn is-primary save"
			aria-keyshortcuts="Enter"
			disabled={!drafts.hasPending || drafts.saving || panel.busy}
			onclick={() => panel.save()}>Ghi điểm <kbd>↵</kbd></button
		>
	</footer>
</div>

<style>
	.board {
		display: flex;
		flex-direction: column;
		gap: 8px;
		height: 100%;
		padding: 8px;
	}
	.cards {
		display: grid;
		flex: 1;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 8px;
		min-height: 0;
	}
	.card {
		--cp-h: var(--cp-h-deck);
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
		min-height: 0;
		padding: 8px 10px 10px;
		border: 1px solid var(--cp-line);
		border-radius: 8px;
		background: var(--cp-surface);
	}
	.card.turn {
		border-color: var(--cp-live);
		box-shadow: inset 0 3px 0 var(--cp-live);
	}
	.card.rung:not(.turn) {
		border-color: var(--cp-armed);
		box-shadow: inset 0 3px 0 var(--cp-armed);
	}
	header {
		display: flex;
		align-items: center;
		gap: 6px;
		min-height: 24px;
	}
	.slot {
		color: var(--cp-muted);
		font-size: 12px;
		font-weight: 600;
	}
	.tag {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		padding: 2px 6px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 700;
		white-space: nowrap;
	}
	.tag :global(svg) {
		width: 12px;
		height: 12px;
	}
	.tag.live {
		background: var(--cp-live-bg);
		color: var(--cp-live-ink);
	}
	.tag.armed {
		background: var(--cp-armed-bg);
		color: var(--cp-armed-ink);
	}
	.lock {
		display: grid;
		place-items: center;
		width: 28px;
		height: 28px;
		margin-left: auto;
		border-radius: 6px;
		color: var(--cp-faint);
		cursor: pointer;
	}
	.lock:hover:not(:disabled) {
		background: var(--cp-hover);
	}
	.lock.on {
		background: var(--cp-bad-bg);
		color: var(--cp-bad-ink);
	}
	.lock :global(svg) {
		width: 16px;
		height: 16px;
	}
	.name {
		overflow: hidden;
		font-size: 15px;
		font-weight: 600;
		line-height: 20px;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.score-row {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.score {
		min-width: 2.2ch;
		font-size: 26px;
		font-weight: 700;
		line-height: 1;
	}
	.board :global(.pending) {
		height: 30px;
		padding: 0 8px;
		border-radius: 6px;
		background: var(--cp-raised);
		color: var(--cp-faint);
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.board :global(.pending.has) {
		background: var(--cp-armed-bg);
		color: var(--cp-armed-ink);
	}
	.nudge {
		--cp-h: 30px;
		display: flex;
		gap: 4px;
		margin-left: auto;
	}
	.nudge .cp-btn {
		padding: 0;
	}
	.nudge :global(svg) {
		width: 15px;
		height: 15px;
	}
	.answer {
		display: flex;
		gap: 6px;
		min-height: 20px;
		color: var(--cp-muted);
		font-size: 14px;
		line-height: 20px;
	}
	.answer .text {
		flex: 1;
		overflow: hidden;
		color: var(--cp-text);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.answer .time {
		flex: none;
		color: var(--cp-faint);
	}
	.verdicts {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 6px;
		margin-top: auto;
	}
	.verdicts .cp-btn {
		font-weight: 600;
	}
	footer {
		--cp-h: 38px;
		display: flex;
		flex: none;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
	}
	.summary {
		display: flex;
		flex: 1 1 120px;
		min-width: 0;
		align-items: center;
		gap: 6px;
		overflow: hidden;
		color: var(--cp-muted);
		font-size: 13px;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.summary .dot {
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: 999px;
		background: var(--cp-armed);
	}
	@media (min-height: 900px) {
		.card {
			gap: 8px;
			padding: 12px 14px 14px;
		}
		.name {
			font-size: 18px;
			line-height: 24px;
		}
		.score {
			font-size: 34px;
		}
		.answer {
			font-size: 16px;
			line-height: 22px;
		}
		.answer .text {
			display: -webkit-box;
			white-space: normal;
			-webkit-line-clamp: 2;
			line-clamp: 2;
			-webkit-box-orient: vertical;
		}
		.verdicts {
			--cp-h: 56px;
		}
		.verdicts .cp-btn {
			font-size: 16px;
		}
	}
	.save kbd {
		font: inherit;
		opacity: 0.7;
	}
</style>
