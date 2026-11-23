<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { socket } from '$lib/socket';

	let { children } = $props();
	let redirecting = false;

	$effect(() => {
		if (!api.auth.user && !redirecting) {
			redirecting = true;
			void goto('/#/').then(() => socket.reconnect());
		}
	});
</script>

{#if api.auth.user}
	{@render children()}
{/if}
