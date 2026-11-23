<script lang="ts">
	import { onMount } from 'svelte';
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { api } from '$lib/api';
	import { socket } from '$lib/socket';
	import { page } from '$app/state';
	import * as Alert from '$lib/components/ui/alert';

	let { children } = $props();
	let runtime = $derived(socket.match.current);
	let stationDisplay = $derived(
		page.route.id?.endsWith('/contestant') ||
			page.route.id?.endsWith('/contestant-next') ||
			page.route.id?.endsWith('/master') ||
			// The new operator panel shows the paused state in its own on-air bar.
			page.route.id?.endsWith('/control-panel-next')
	);

	onMount(() => {
		const heartbeat = window.setInterval(() => {
			if (api.auth.user) void api.auth.refresh().catch(() => undefined);
		}, 30_000);
		return () => window.clearInterval(heartbeat);
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

{#if runtime?.status === 'paused' && !stationDisplay}
	<div class="pointer-events-none fixed inset-x-4 top-4 mx-auto max-w-xl">
		<Alert.Root variant="destructive"
			><Alert.Title>Trận đấu đang tạm dừng</Alert.Title><Alert.Description
				>Mọi thao tác thi đấu và đầu vào thí sinh đã bị khóa.</Alert.Description
			></Alert.Root
		>
	</div>
{/if}
{@render children()}
