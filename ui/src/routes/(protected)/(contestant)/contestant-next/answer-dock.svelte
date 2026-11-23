<script lang="ts">
	import { tick } from 'svelte';
	import { socket } from '$lib/socket';
	import type { MatchRuntimeState, RuntimeContestant } from '$lib/socket/opsocket';
	import { looksLikeRawTelex, normalizeAnswer } from '$lib/station/answer';

	let {
		open,
		runtime,
		self
	}: { open: boolean; runtime: MatchRuntimeState | null; self: RuntimeContestant | undefined } = $props();

	let input = $state<HTMLInputElement>();
	/** Read-only mirror of what is typed; the input value itself is never rewritten while typing. */
	let draft = $state('');
	let busy = $state(false);
	let error = $state('');
	let windowKey = $derived(runtime ? `${runtime.match_id}:${runtime.epoch}:${runtime.input_epoch}` : '');
	let lastWindow = '';
	let telexHint = $derived(open && looksLikeRawTelex(draft));
	let placeholder = $derived(
		!runtime
			? 'Chờ trận đấu'
			: runtime.status === 'paused'
				? 'Trận đấu đang tạm dừng'
				: self?.locked
					? 'Đầu vào đã khóa'
					: open
						? 'Nhập câu trả lời, ENTER để gửi'
						: ''
	);

	$effect(() => {
		// A new question clears the box and any old error.
		if (windowKey === lastWindow) return;
		lastWindow = windowKey;
		if (input) input.value = '';
		draft = '';
		error = '';
	});

	$effect(() => {
		if (open && !busy) void tick().then(() => input?.focus({ preventScroll: true }));
	});

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (!open || busy || !input) return;
		// Read the DOM value: input methods may not emit a final input event.
		const value = normalizeAnswer(input.value);
		if (!value) {
			error = 'Chưa nhập câu trả lời';
			return;
		}
		const key = windowKey;
		busy = true;
		error = '';
		try {
			await socket.contestant.submitAnswer(value);
			if (key === windowKey && input) {
				input.value = '';
				draft = '';
			}
		} catch (cause) {
			if (key !== windowKey || (cause instanceof DOMException && cause.name === 'AbortError')) return;
			error = cause instanceof Error ? cause.message : 'Không thể gửi câu trả lời';
		} finally {
			busy = false;
		}
	}
</script>

<form class="dock" class:open onsubmit={submit} aria-label="Gửi câu trả lời">
	<input
		bind:this={input}
		type="text"
		lang="vi"
		translate="no"
		spellcheck="false"
		autocomplete="off"
		autocapitalize="characters"
		maxlength="1000"
		aria-label="Câu trả lời của bạn"
		{placeholder}
		disabled={!open}
		oninput={(event) => (draft = event.currentTarget.value)}
	/>
	<p class="note" class:error={Boolean(error)} class:hint={!error && telexHint} role={error ? 'alert' : 'status'}>
		{#if error}{error}{:else if busy}Đang gửi…{:else if telexHint}Có vẻ chưa bật gõ tiếng Việt — kiểm tra trước khi gửi{:else if open}Gửi lại để thay câu trả lời đã ghi{/if}
	</p>
</form>

<style>
	.dock {
		display: grid;
		grid-template-rows: auto auto;
		padding: 0.6em 0.9em 0.5em;
		border-top: 1px solid var(--line);
		background: var(--raised);
	}
	.dock.open {
		background: #fff;
		box-shadow: inset 0 3px 0 var(--text);
	}
	input {
		width: 100%;
		min-width: 0;
		padding: 0.25em 0.4em;
		border: 2px solid var(--line);
		border-radius: 8px;
		background: #fff;
		font-size: 2em;
		font-weight: 600;
		text-transform: uppercase;
	}
	input:disabled {
		background: transparent;
		border-color: transparent;
		color: var(--faint);
	}
	input::placeholder {
		color: var(--faint);
		font-weight: 500;
		text-transform: none;
	}
	input:focus-visible {
		outline: none;
		border-color: var(--text);
	}
	.note {
		min-height: 1.4em;
		padding-top: 0.25em;
		color: var(--muted);
		font-size: 0.75em;
	}
	.note.hint {
		color: #92400e;
		font-weight: 700;
	}
	.note.error {
		color: #b91c1c;
		font-weight: 700;
	}
</style>
