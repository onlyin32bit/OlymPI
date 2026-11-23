<script lang="ts">
	import { socket } from '$lib/socket';
	import type { BellView } from '$lib/station/view';
	import BellIcon from '@lucide/svelte/icons/bell';
	import BellOff from '@lucide/svelte/icons/bell-off';
	import BellRing from '@lucide/svelte/icons/bell-ring';

	let { bell }: { bell: BellView } = $props();
	let busy = $state(false);
	let error = $state('');

	/**
	 * Rings on press-down, not release, and only from a pointer: the bell is a
	 * screen button, so no key (including Enter in the answer box) can press it.
	 */
	async function press(event: PointerEvent): Promise<void> {
		if (event.button !== 0 || bell.state !== 'open' || busy) return;
		event.preventDefault();
		busy = true;
		error = '';
		try {
			await socket.contestant.ring();
		} catch (cause) {
			if (!(cause instanceof DOMException && cause.name === 'AbortError'))
				error = cause instanceof Error ? cause.message : 'Không thể bấm chuông';
		} finally {
			busy = false;
		}
	}
</script>

<div class="cell">
	<button
		type="button"
		class={`bell ${bell.state}`}
		tabindex="-1"
		aria-disabled={bell.state !== 'open'}
		aria-label={bell.label}
		onpointerdown={press}
		oncontextmenu={(event) => event.preventDefault()}
	>
		{#if bell.state === 'open'}<BellIcon />{:else if bell.state === 'rung'}<BellRing />{:else}<BellOff />{/if}
		<span>{busy ? 'Đang gửi…' : bell.label}</span>
	</button>
	{#if error}<p class="error" role="alert">{error}</p>{/if}
</div>

<style>
	.cell {
		display: flex;
		flex-direction: column;
		gap: 0.4em;
		min-height: 0;
		padding: 0.8em;
		border-top: 1px solid var(--line);
	}
	.bell {
		display: flex;
		flex: 1;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.4em;
		min-height: 0;
		border: 2px solid var(--line);
		border-radius: 12px;
		background: var(--raised);
		color: var(--faint);
		font-size: 1.35em;
		font-weight: 800;
		letter-spacing: 0.02em;
		text-transform: uppercase;
		touch-action: manipulation;
		user-select: none;
		cursor: not-allowed;
		transition:
			background-color 120ms ease,
			transform 80ms ease;
	}
	.bell :global(svg) {
		width: 1.8em;
		height: 1.8em;
	}
	.bell.open {
		border-color: #b91c1c;
		background: #dc2626;
		color: #fff;
		cursor: pointer;
	}
	.bell.open:active {
		transform: scale(0.98);
	}
	.bell.rung {
		border-color: #ca8a04;
		background: #fef08a;
		color: #713f12;
	}
	.bell.eliminated {
		color: #b91c1c;
		text-decoration: line-through;
	}
	.error {
		color: #b91c1c;
		font-size: 0.75em;
	}
	@media (prefers-reduced-motion: reduce) {
		.bell {
			transition: none;
		}
		.bell.open:active {
			transform: none;
		}
	}
</style>
