<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Spinner } from '$lib/components/ui/spinner';
	import * as Table from '$lib/components/ui/table';
	import { socket } from '$lib/socket';
	import type { Match, MatchEvent } from '$lib/types';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';

	const matchID = page.params.id ?? '';
	let match = $state<Match | null>(null);
	let events = $state<MatchEvent[]>([]);
	let loading = $state(true);
	let refreshing = $state(false);
	let reloadQueued = false;
	let wasCurrent = false;
	let error = $state('');
	let newestFirst = $derived(events.slice().reverse());

	onMount(() => {
		void load();
		wasCurrent = socket.match.current?.match_id === matchID;
		return socket.match.onUpdate((runtime) => {
			const isCurrent = runtime?.match_id === matchID;
			if (isCurrent || wasCurrent) void load();
			wasCurrent = isCurrent;
		});
	});

	async function load(): Promise<void> {
		if (refreshing) {
			reloadQueued = true;
			return;
		}
		refreshing = true;
		do {
			reloadQueued = false;
			try {
				[match, events] = await Promise.all([
					api.matches.get(matchID),
					api.matches.events(matchID)
				]);
				error = '';
			} catch (cause) {
				error = cause instanceof Error ? cause.message : 'Không thể tải nhật ký trận';
			} finally {
				loading = false;
			}
		} while (reloadQueued);
		refreshing = false;
	}

	function payload(value: unknown): string {
		return JSON.stringify(value ?? {}, null, 2);
	}
</script>

<svelte:head><title>Nhật ký {match?.name ?? 'trận đấu'} - {brand.appName}</title></svelte:head>

<header class="flex flex-wrap items-end justify-between gap-4">
	<div class="flex flex-col gap-3">
		<Button variant="ghost" href="#/match" class="w-fit"
			><ArrowLeft data-icon="inline-start" />Danh sách trận</Button
		>
		<div>
			<h1 class="text-3xl font-bold tracking-tight">{match?.name ?? 'Nhật ký trận đấu'}</h1>
			{#if match}<div class="mt-2 flex flex-wrap gap-2">
					<Badge variant="outline">Mùa {match.season}</Badge>
					<Badge variant="outline">{match.stage}</Badge>
					{#if match.selected}<Badge>Đang được tải</Badge>{/if}
				</div>{/if}
		</div>
	</div>
	<Button variant="outline" disabled={refreshing} onclick={load}
		><RefreshCw data-icon="inline-start" />Làm mới</Button
	>
</header>

{#if error}<Alert.Root variant="destructive">
		<Alert.Title>Không thể tải nhật ký</Alert.Title>
		<Alert.Description>{error}</Alert.Description>
	</Alert.Root>{/if}

<Card.Root>
	<Card.Header>
		<Card.Title>Sự kiện trận đấu</Card.Title>
		<Card.Description>{events.length} sự kiện và snapshot đã lưu</Card.Description>
	</Card.Header>
	<Card.Content class="p-0">
		{#if loading}
			<div class="flex h-48 items-center justify-center"><Spinner /></div>
		{:else if newestFirst.length === 0}
			<p class="p-8 text-center text-muted-foreground">Trận chưa có sự kiện.</p>
		{:else}
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head class="w-20">STT</Table.Head>
						<Table.Head class="w-44">Thời gian</Table.Head>
						<Table.Head>Sự kiện</Table.Head>
						<Table.Head class="w-32">Người thực hiện</Table.Head>
						<Table.Head class="w-32 text-right">Dữ liệu</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each newestFirst as event (`${event.match_id}-${event.sequence}`)}
						<Table.Row>
							<Table.Cell class="font-mono tabular-nums">#{event.sequence}</Table.Cell>
							<Table.Cell class="font-mono text-xs tabular-nums">
								{new Date(event.created_at).toLocaleString('vi-VN', { hour12: false })}
							</Table.Cell>
							<Table.Cell class="font-medium">{event.type}</Table.Cell>
							<Table.Cell>{event.actor_id ? `#${event.actor_id}` : 'Hệ thống'}</Table.Cell>
							<Table.Cell class="text-right">
								<details class="inline-block text-left">
									<summary class="cursor-pointer text-sm font-medium">Xem</summary>
									<pre
										class="mt-2 max-h-72 w-96 overflow-auto rounded-md bg-muted p-3 text-xs whitespace-pre-wrap">{payload(
											event.payload
										)}</pre>
								</details>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		{/if}
	</Card.Content>
</Card.Root>
