<script lang="ts">
	import { Display } from '$lib/components/display';
	import SoundOutput from '$lib/components/display/sound-output.svelte';
	import { socket } from '$lib/socket';

	// The TV is always the audience view, even in a browser logged in as operator.
	$effect(() => socket.usePublicView());
</script>

<div class="relative h-svh w-screen overflow-hidden bg-black">
	<Display fullScreen />
	<SoundOutput />
</div>
