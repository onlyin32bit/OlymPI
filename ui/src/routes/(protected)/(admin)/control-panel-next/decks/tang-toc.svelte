<script lang="ts">
	import { panel } from '$lib/operator/panel.svelte';
	import { SLOTS, batchPoints } from '$lib/operator/scoring';

	let liveKey = $derived(panel.drafts.liveKey);
	let batch = $derived(panel.drafts.batch(liveKey));
	let points = $derived(batch ? batchPoints(batch, panel.drafts.rules) : undefined);
	/** Answers in the order they arrived; contestants without an answer go last. */
	let rows = $derived(
		SLOTS.map((slot) => ({ slot, entry: panel.runtime?.contestants.find((value) => value.slot === slot) }))
			.map((row) => ({ ...row, time: row.entry?.answer && row.entry.time_ms > 0 ? row.entry.time_ms : Number.POSITIVE_INFINITY }))
			.sort((a, b) => a.time - b.time || a.slot - b.slot)
	);
</script>

<table class="ranking">
	<thead>
		<tr><th>Thứ tự</th><th>Thí sinh</th><th>Câu trả lời</th><th class="num">Thời gian</th><th class="num">Điểm</th></tr>
	</thead>
	<tbody>
		{#each rows as row, index (row.slot)}
			{@const value = batch?.verdicts[row.slot]}
			<tr class:correct={value === 'correct'} class:wrong={value === 'wrong'}>
				<td class="cp-num">{Number.isFinite(row.time) ? index + 1 : '—'}</td>
				<td class="name">{row.entry?.name || `TS${row.slot}`}</td>
				<td class="answer" title={row.entry?.answer}>{row.entry?.answer || '—'}</td>
				<td class="num cp-num">{Number.isFinite(row.time) ? `${(row.time / 1000).toFixed(2)}s` : '—'}</td>
				<td class="num cp-num points">{value ? (points?.[row.slot] ?? 0) : '—'}</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	.ranking {
		width: 100%;
		border-collapse: collapse;
		font-size: 15px;
	}
	th {
		padding: 0 8px 6px;
		color: var(--cp-muted);
		font-size: 12px;
		font-weight: 500;
		text-align: left;
	}
	td {
		height: var(--cp-h);
		padding: 0 8px;
		border-top: 1px solid var(--cp-line);
	}
	.num {
		text-align: right;
	}
	.name {
		font-weight: 600;
		white-space: nowrap;
	}
	.answer {
		overflow: hidden;
		max-width: 1px;
		width: 45%;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.points {
		font-weight: 700;
	}
	tr.correct .points {
		color: var(--cp-ok);
	}
	tr.wrong .answer,
	tr.wrong .points {
		color: var(--cp-faint);
	}
	tr.wrong .answer {
		text-decoration: line-through;
	}
</style>
