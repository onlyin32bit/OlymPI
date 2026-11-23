<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { Logo } from '$lib/components/common';
	import { Display } from '$lib/components/display';
	import * as Tabs from '$lib/components/ui/tabs';
	import { APP_NAME } from '$lib/constant';
	import { socket } from '$lib/socket';
	import { phaseFor } from '$lib/display/phase-manifest';
	import type { Sound } from '$lib/types';
	import ContestantTable from './contestant-table.svelte';
	import LogPanel from '$lib/components/operator/log-panel.svelte';
	import OperatorConsole from './operator-console.svelte';
	import Soundboard from '$lib/components/operator/soundboard.svelte';

	let controller = $derived(socket.controller.current);
	let activePhase = $derived(phaseFor(controller.phase));
	let matchRuntime = $derived(socket.match.current);
	let currentScreenLabel = $derived(
		activePhase.screens.find((screen) => screen.id === controller.screen)?.label ??
			controller.screen
	);
	let detailTab = $state('1');
	let sounds = $state<Sound[]>([]);

	let visibleSounds = $derived(sounds.filter((sound) => sound.enabled));

	onMount(() => {
		void loadSounds();
		return socket.sound.onCatalogUpdated(() => void loadSounds());
	});

	async function loadSounds(): Promise<void> {
		try {
			sounds = await api.sounds.manage();
		} catch {
			sounds = [];
		}
	}
	async function finishVeDichVideo(questionID: string, sessionID: number): Promise<void> {
		const current = socket.match.current;
		const video = current?.controller.ve_dich_video;
		if (current?.current_question?.id !== questionID || video?.session_id !== sessionID) return;
		await socket.controller.veDichVideo('finish', {
			question_id: questionID,
			session_id: sessionID
		});
	}
</script>

<svelte:head>
	<title>Control Panel - {APP_NAME}</title>
</svelte:head>

<main
	class="grid h-screen w-screen grid-cols-[0.8fr_1fr] grid-rows-[50px_minmax(0,1fr)_minmax(0,1fr)] gap-1.5 overflow-hidden bg-blue-100 p-1.5"
>
	<div class="col-span-2 flex items-center gap-3 rounded-md bg-white px-2 py-1">
		<Logo color="blue" class="h-8" />
		<h1 class="text-lg font-bold">Control Panel - {APP_NAME}</h1>
	</div>
	<LogPanel />
	<div class="min-h-0 min-w-0 overflow-hidden rounded-md bg-white p-2 transition-shadow">
		<Tabs.Root bind:value={detailTab} class="flex h-full min-h-0 flex-col">
			<Tabs.List class="w-full bg-slate-200 p-1 shadow-2xs">
				{#each [{ value: '1', label: 'Thí Sinh' }, { value: '4', label: 'Soundboard' }, { value: '2', label: 'Status' }, { value: '3', label: 'Settings' }] as item (item.value)}
					<Tabs.Trigger
						class="text-slate-400 data-[state=active]:font-bold data-[state=active]:text-slate-800 data-[state=active]:shadow-none"
						value={item.value}>{item.label}</Tabs.Trigger
					>
				{/each}
			</Tabs.List>
			<Tabs.Content class="flex min-h-0 flex-1 animate-in flex-col gap-2 zoom-in-95" value="1">
				<ContestantTable />
			</Tabs.Content>
			<Tabs.Content class="min-h-0 flex-1 overflow-hidden" value="4">
				<Soundboard
					sounds={visibleSounds}
					onUpdated={(updated) =>
						(sounds = sounds.map((sound) => (sound.id === updated.id ? updated : sound)))}
				/>
			</Tabs.Content>
			<Tabs.Content class="min-h-0 flex-1 overflow-auto p-3" value="2">
				<dl class="grid gap-4 text-sm">
					<div>
						<dt class="text-xs font-bold text-muted-foreground uppercase">Trận đấu</dt>
						<dd class="font-medium">{matchRuntime?.match_name ?? 'Chưa chọn trận đấu'}</dd>
					</div>
					<div>
						<dt class="text-xs font-bold text-muted-foreground uppercase">Trạng thái</dt>
						<dd class="font-medium">{matchRuntime?.status ?? 'Chưa có trận đang tải'}</dd>
					</div>
					<div>
						<dt class="text-xs font-bold text-muted-foreground uppercase">Vòng hiện tại</dt>
						<dd class="font-medium">{activePhase.label}</dd>
					</div>
					<div>
						<dt class="text-xs font-bold text-muted-foreground uppercase">Màn hình hiện tại</dt>
						<dd class="font-medium">{currentScreenLabel}</dd>
					</div>
					<div>
						<dt class="text-xs font-bold text-muted-foreground uppercase">Chuông</dt>
						<dd class="font-medium">{controller.is_allowing_bell ? 'Đang mở' : 'Đang khóa'}</dd>
					</div>
				</dl>
			</Tabs.Content>
			<Tabs.Content value="3"></Tabs.Content>
		</Tabs.Root>
	</div>
	<div class="overflow-hidden rounded-md">
		<Display preview onVeDichVideoFinished={finishVeDichVideo} />
	</div>
	<OperatorConsole />
</main>
