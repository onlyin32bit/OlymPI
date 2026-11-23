<script lang="ts">
	import { api } from '$lib/api';
	import { vcnvRevealKey, type VCNVView } from '$lib/display/types';
	import { socket } from '$lib/socket';
	import type { MatchRuntimeState } from '$lib/socket/opsocket';

	let { runtime }: { runtime: MatchRuntimeState | null } = $props();

	const STATE_LABELS: Record<string, string> = { u: 'Chưa mở', s: 'Đang mở', c: 'Đúng', w: 'Sai' };
	/** Picture covers in the board's own order: four corners, then the centre on top. */
	const COVERS = [
		{ index: 0, area: 'top-left' },
		{ index: 1, area: 'top-right' },
		{ index: 2, area: 'bottom-left' },
		{ index: 3, area: 'bottom-right' },
		{ index: 4, area: 'center' }
	];

	let board = $state<VCNVView | null>(null);
	let revealKey = $derived(vcnvRevealKey(runtime));
	let rows = $derived(runtime?.controller.vcnv_state.row_states ?? ['u', 'u', 'u', 'u']);
	let tiles = $derived(runtime?.controller.vcnv_state.image_open_state ?? [false, false, false, false, false]);
	let currentRow = $derived(Number(/^vcnv\.rows\.([0-3])$/.exec(runtime?.controller.selected_question_id ?? '')?.[1] ?? -1));

	// The endpoint redacts words by reveal state, so it is refetched whenever that changes.
	$effect(() => {
		const key = revealKey;
		if (!key) {
			board = null;
			return;
		}
		const request = new AbortController();
		void api.public
			.vcnv({ signal: request.signal })
			.then((value) => {
				if (!request.signal.aborted && vcnvRevealKey(socket.match.current) === key) board = value;
			})
			.catch(() => undefined);
		return () => request.abort();
	});
</script>

<div class="board">
	<!-- Each opened piece carries only its own window of the picture; closed pieces carry none,
	     so the hidden parts are never in the DOM and cannot be dragged or saved from here. -->
	<figure class="picture" aria-label="Hình ảnh chướng ngại vật" oncontextmenu={(event) => event.preventDefault()} role="presentation">
		{#each COVERS as cover (cover.index)}
			{#if tiles[cover.index]}
				<div class={`piece ${cover.area}`}>
					{#if board?.obstacle_asset_id}
						<img src={`/api/assets/${board.obstacle_asset_id}`} alt="" draggable="false" />
					{/if}
				</div>
			{:else}
				<span class={`cover ${cover.area}`} aria-hidden="true">{cover.index === 4 ? 'TT' : cover.index + 1}</span>
			{/if}
		{/each}
	</figure>
	<div class="rows">
		{#each [0, 1, 2, 3] as index (index)}
			{@const state = rows[index] ?? 'u'}
			{@const word = (board?.row_answers[index] ?? '').replace(/\s/g, '')}
			<div class={`row state-${state}`} class:current={index === currentRow}>
				<span class="label">Hàng {index + 1}</span>
				<span class="letters" aria-label={`${board?.row_lengths[index] ?? 0} chữ cái, ${STATE_LABELS[state]}`}>
					{#each Array.from({ length: board?.row_lengths[index] ?? 0 }, (_, position) => position) as letter (letter)}
						<span class="box">{word[letter] ?? ''}</span>
					{/each}
				</span>
			</div>
		{/each}
		<p class="keyword">
			<span>Chướng ngại vật</span>
			{#if board?.obstacle}<strong>{board.obstacle}</strong>{:else}<strong>{board?.obstacle_length ?? 0} chữ cái</strong>{/if}
		</p>
	</div>
</div>

<style>
	.board {
		display: grid;
		flex: none;
		grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
		gap: 1em;
	}
	.picture {
		position: relative;
		aspect-ratio: 16 / 10;
		overflow: hidden;
		border-radius: 8px;
		background: var(--raised);
	}
	.picture {
		user-select: none;
	}
	/* A piece is a window onto the full picture: its image is scaled to the whole frame and offset. */
	.piece {
		position: absolute;
		overflow: hidden;
	}
	.piece img {
		position: absolute;
		left: 0;
		top: 0;
		width: 200%;
		height: 200%;
		max-width: none;
		object-fit: cover;
		pointer-events: none;
	}
	.piece.top-right img {
		left: -100%;
	}
	.piece.bottom-left img {
		top: -100%;
	}
	.piece.bottom-right img {
		left: -100%;
		top: -100%;
	}
	.piece.center img {
		width: 250%;
		height: 250%;
		left: -75%;
		top: -75%;
	}
	.cover {
		position: absolute;
		display: grid;
		place-items: center;
		background: #3f3f46;
		color: #fafafa;
		font-weight: 800;
		outline: 2px solid #fff;
	}
	.top-left {
		inset: 0 50% 50% 0;
	}
	.top-right {
		inset: 0 0 50% 50%;
	}
	.bottom-left {
		inset: 50% 50% 0 0;
	}
	.bottom-right {
		inset: 50% 0 0 50%;
	}
	.center {
		inset: 30% 30% 30% 30%;
		background: #52525b;
	}
	.rows {
		display: flex;
		flex-direction: column;
		gap: 0.45em;
		min-width: 0;
	}
	.row {
		display: flex;
		align-items: center;
		gap: 0.6em;
		padding: 0.25em 0.4em;
		border-radius: 6px;
	}
	.row.current {
		box-shadow: inset 0 0 0 2px var(--text);
	}
	.label {
		flex: none;
		width: 4.4em;
		color: var(--muted);
		font-size: 0.85em;
		font-weight: 600;
	}
	.letters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.2em;
	}
	.box {
		display: grid;
		place-items: center;
		width: 1.35em;
		height: 1.35em;
		border: 1.5px solid var(--line);
		border-radius: 4px;
		background: #fff;
		font-weight: 800;
	}
	.state-s .box {
		border-color: var(--text);
	}
	.state-c .box {
		border-color: #16a34a;
		background: #dcfce7;
	}
	.state-w .box {
		background: var(--raised);
		color: var(--faint);
	}
	.keyword {
		display: flex;
		align-items: baseline;
		gap: 0.6em;
		margin-top: 0.3em;
		color: var(--muted);
		font-weight: 600;
	}
	.keyword strong {
		color: var(--text);
		font-size: 1.2em;
	}
</style>
