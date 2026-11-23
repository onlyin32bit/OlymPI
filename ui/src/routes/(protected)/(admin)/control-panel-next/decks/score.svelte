<script lang="ts">
	import { panel } from '$lib/operator/panel.svelte';
	import { socket } from '$lib/socket';
	import Music from '@lucide/svelte/icons/music';

	let ranked = $derived([...(panel.runtime?.contestants ?? [])].sort((a, b) => b.score - a.score || a.slot - b.slot));
</script>

<div class="score-deck">
	<button
		type="button"
		class="cp-btn"
		disabled={panel.busy}
		onclick={() => void panel.run(() => socket.sound.play('builtin.main.pointsummary'))}><Music />Nhạc tổng kết điểm</button
	>
	<ol>
		{#each ranked as entry (entry.slot)}
			<li><span class="name">{entry.name || `TS${entry.slot}`}</span><span class="cp-num">{entry.score}</span></li>
		{/each}
	</ol>
</div>

<style>
	.score-deck {
		display: flex;
		align-items: flex-start;
		gap: 16px;
	}
	ol {
		display: grid;
		flex: 1;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 6px;
	}
	li {
		display: flex;
		justify-content: space-between;
		gap: 8px;
		padding: 10px 12px;
		border-radius: 6px;
		background: var(--cp-raised);
		font-size: 15px;
	}
	.name {
		overflow: hidden;
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.cp-num {
		font-weight: 700;
	}
</style>
