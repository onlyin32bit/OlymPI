<script lang="ts" module>
	import { socket } from '$lib/socket';

	// The legacy loader shows until the first runtime/controller state arrives. `onUpdate` replays
	// the current value once on subscribe, so a null replay alone does not count as arrival.
	let calls = 0;
	const subscribable = typeof window !== 'undefined' && typeof socket.match?.onUpdate === 'function';
	let arrived = $state(!subscribable);
	if (subscribable)
		socket.match.onUpdate((runtime) => {
			calls++;
			if (runtime || calls > 1) arrived = true;
		});
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api';
	import {
		emptySiteSettings,
		publicView,
		vcnvRevealKey,
		type DisplayContext,
		type VCNVView
	} from '$lib/display/types';
	import { DEFAULT_SCORING } from '$lib/operator/scoring';
	import DisplayFrame from './display-frame.svelte';
	import DisplaySceneRouter from './display-scene-router.svelte';

	type Props = {
		fullScreen?: boolean;
		/** Operator preview: public view of the runtime, no audio, muted media. */
		preview?: boolean;
		onVeDichVideoFinished?: (questionID: string, sessionID: number) => void | Promise<unknown>;
	};

	let { fullScreen = false, preview = false, onVeDichVideoFinished }: Props = $props();

	let vcnv = $state<VCNVView | null>(null);
	let vcnvSource = '';
	let runtime = $derived(socket.match.current);
	let revealKey = $derived(vcnvRevealKey(runtime));
	let context = $derived<DisplayContext>({
		match: preview ? publicView(runtime) : runtime,
		site: api.settings.current ?? { ...emptySiteSettings, scoring: DEFAULT_SCORING },
		phase: socket.controller.current.phase,
		screen: socket.controller.current.screen,
		vcnv,
		preview,
		onVeDichVideoFinished
	});

	// Settings reach the TV even with sound output off: re-fetch on Settings.Updated and after
	// every reconnect (an update may have been missed). Loads retry until they succeed.
	$effect(() => {
		let request: AbortController | undefined;
		const refresh = (): void => {
			request?.abort();
			request = new AbortController();
			void api.settings.load({ signal: request.signal }).catch(() => undefined);
		};
		const offUpdated = socket.onSettingsUpdated(refresh);
		const offOpen = socket.onOpen(refresh);
		if (!socket.connected) refresh();
		return () => {
			offUpdated();
			offOpen();
			request?.abort();
		};
	});

	// Keep the previous board while a reveal refetches (no blink); retry with backoff until the
	// reveal key changes. Data from another match/question set is dropped at once.
	$effect(() => {
		const key = revealKey;
		const current = untrack(() => socket.match.current);
		const source = key ? JSON.stringify([current?.match_id, current?.question_revision_id]) : '';
		if (source !== vcnvSource) {
			vcnvSource = source;
			vcnv = null;
		}
		if (!key) return;
		const request = new AbortController();
		let retry: ReturnType<typeof setTimeout> | undefined;
		let delay = 500;
		const load = (): void => {
			void api.public
				.vcnv({ signal: request.signal })
				.then((value) => {
					if (!request.signal.aborted && vcnvRevealKey(socket.match.current) === key) vcnv = value;
				})
				.catch((cause) => {
					if (request.signal.aborted || vcnvRevealKey(socket.match.current) !== key) return;
					console.warn('Unable to load VCNV display data; retrying', cause);
					retry = setTimeout(load, delay);
					delay = Math.min(delay * 2, 10_000);
				});
		};
		load();
		return () => {
			request.abort();
			clearTimeout(retry);
		};
	});
</script>

<DisplayFrame {fullScreen} interactive={fullScreen || Boolean(onVeDichVideoFinished)}>
	<DisplaySceneRouter {context} loading={!arrived} />
</DisplayFrame>
