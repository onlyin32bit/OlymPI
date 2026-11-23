<script lang="ts">
	import DisplayFrame from '$lib/components/display/display-frame.svelte';
	import Score from '$lib/components/display/score/score.svelte';
	import { emptySiteSettings, publicView, type DisplayContext } from '$lib/display/types';
	import { api } from '$lib/api';
	import { DEFAULT_SCORING } from '$lib/operator/scoring';
	import { socket } from '$lib/socket';

	// Score board only: no scene router and no sound output, so it never plays audio.
	// Live score and stage updates arrive through the socket; the public view hides operator data.
	$effect(() => socket.usePublicView());

	let context = $derived<DisplayContext>({
		match: publicView(socket.match.current),
		site: api.settings.current ?? { ...emptySiteSettings, scoring: DEFAULT_SCORING },
		phase: 'score',
		screen: 'score',
		vcnv: null,
		preview: false
	});
</script>

<svelte:head><title>Bảng điểm</title></svelte:head>

<div class="relative h-svh w-screen overflow-hidden bg-black">
	<DisplayFrame fullScreen>
		<Score {context} />
	</DisplayFrame>
</div>
