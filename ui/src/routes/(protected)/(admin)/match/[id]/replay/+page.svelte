<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Match, MatchEvent } from '$lib/types';
	import type { MatchRuntimeState } from '$lib/socket/opsocket';
	import * as Alert from '$lib/components/ui/alert';
	import * as Card from '$lib/components/ui/card';
	import * as Field from '$lib/components/ui/field';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Spinner } from '$lib/components/ui/spinner';
	import { Textarea } from '$lib/components/ui/textarea';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import Pause from '@lucide/svelte/icons/pause';
	import Play from '@lucide/svelte/icons/play';
	import SkipBack from '@lucide/svelte/icons/skip-back';
	import SkipForward from '@lucide/svelte/icons/skip-forward';

	const matchID = page.params.id ?? '';
	let match = $state<Match | null>(null);
	let events = $state<MatchEvent[]>([]);
	let index = $state(0);
	let loading = $state(true);
	let playing = $state(false);
	let speed = $state(1);
	let resultText = $state('');
	let amendmentReason = $state('');
	let error = $state('');
	let saving = $state(false);
	let timer: ReturnType<typeof setInterval> | null = null;
	let current = $derived(events[index] ?? null);
	let replayState = $derived((current?.state ?? null) as MatchRuntimeState | null);
	let contestants = $derived(replayState?.contestants ?? match?.contestants ?? []);

	onMount(() => {
		void load();
		return stop;
	});
	async function load() {
		try {
			[match, events] = await Promise.all([api.matches.get(matchID), api.matches.events(matchID)]);
			resultText = JSON.stringify(match.result ?? {}, null, 2);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Không thể tải lịch sử trận';
		} finally {
			loading = false;
		}
	}
	function toggle() {
		if (playing) stop();
		else {
			playing = true;
			timer = setInterval(() => {
				if (index >= events.length - 1) {
					stop();
					return;
				}
				index++;
			}, 1000 / speed);
		}
	}
	function stop() {
		playing = false;
		if (timer) clearInterval(timer);
		timer = null;
	}
	async function amend(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		error = '';
		try {
			await api.matches.amendResult(matchID, JSON.parse(resultText), amendmentReason);
			amendmentReason = '';
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Không thể sửa kết quả';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>Phát lại {match?.name ?? ''} - {brand.appName}</title></svelte:head>
<header class="flex flex-col gap-4">
	<Button variant="ghost" href="#/match" class="w-fit"
		><ArrowLeft data-icon="inline-start" />Danh sách trận</Button
	>
	<div>
		<p class="text-sm font-medium text-muted-foreground">
			Xem lại riêng tư · không phát lên màn hình trực tiếp
		</p>
		<h1 class="text-3xl font-bold tracking-tight">{match?.name ?? 'Phát lại trận đấu'}</h1>
	</div>
</header>
{#if error}<Alert.Root variant="destructive"
		><Alert.Title>Không thể hoàn thành</Alert.Title><Alert.Description>{error}</Alert.Description
		></Alert.Root
	>{/if}
{#if loading}<div class="flex h-64 items-center justify-center"><Spinner /></div>{:else}
	<div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
		<Card.Root
			><Card.Header
				><Card.Title>Trạng thái tại sự kiện {current?.sequence ?? 0}</Card.Title><Card.Description
					>{current?.type ?? 'Chưa có sự kiện'} · {current
						? new Date(current.created_at).toLocaleString('vi-VN')
						: ''}</Card.Description
				></Card.Header
			><Card.Content
				><div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
					{#each contestants as contestant (contestant.slot)}<div class="rounded-lg border p-4">
							<p class="font-semibold">{contestant.name}</p>
							<p class="text-sm text-muted-foreground">{contestant.class}</p>
							{#if 'score' in contestant}<p class="mt-3 text-3xl font-bold tabular-nums">
									{contestant.score}
								</p>
								<p class="text-sm">{contestant.answer || 'Chưa trả lời'}</p>{/if}
						</div>{/each}
				</div>
				<div class="mt-6 rounded-lg bg-muted p-4">
					<div class="flex flex-wrap gap-2">
						<Badge>{replayState?.status ?? match?.status}</Badge><Badge variant="outline"
							>{replayState?.controller?.phase ?? 'idle'}</Badge
						><Badge variant="outline">{replayState?.controller?.screen ?? 'intro'}</Badge>
					</div>
					<pre class="mt-4 max-h-80 overflow-auto text-xs">{JSON.stringify(
							current?.payload ?? {},
							null,
							2
						)}</pre>
				</div></Card.Content
			><Card.Footer class="flex-col gap-4"
				><Input
					aria-label="Dòng thời gian"
					type="range"
					min={0}
					max={Math.max(0, events.length - 1)}
					bind:value={index}
				/>
				<div class="flex flex-wrap items-center justify-center gap-2">
					<Button
						size="icon"
						variant="outline"
						aria-label="Sự kiện đầu"
						onclick={() => {
							stop();
							index = 0;
						}}><SkipBack /></Button
					><Button onclick={toggle}
						>{#if playing}<Pause data-icon="inline-start" />Tạm dừng{:else}<Play
								data-icon="inline-start"
							/>Phát{/if}</Button
					><Button
						size="icon"
						variant="outline"
						aria-label="Sự kiện cuối"
						onclick={() => {
							stop();
							index = Math.max(0, events.length - 1);
						}}><SkipForward /></Button
					><select
						class="h-9 rounded-md border bg-background px-3 text-sm"
						bind:value={speed}
						onchange={() => {
							if (playing) {
								stop();
								toggle();
							}
						}}
						><option value={0.5}>0.5×</option><option value={1}>1×</option><option value={2}
							>2×</option
						><option value={4}>4×</option></select
					>
				</div></Card.Footer
			></Card.Root
		>
		<div class="flex flex-col gap-6">
			<Card.Root
				><Card.Header
					><Card.Title>Kết quả chính thức</Card.Title><Card.Description
						>Sửa sau khi hoàn thành luôn cần lý do và được ghi kiểm toán.</Card.Description
					></Card.Header
				><Card.Content
					><form onsubmit={amend}>
						<Field.FieldGroup
							><Field.Field
								><Field.FieldLabel for="result">Kết quả JSON</Field.FieldLabel><Textarea
									id="result"
									class="min-h-40 font-mono text-xs"
									bind:value={resultText}
									disabled={match?.status !== 'completed'}
								/></Field.Field
							><Field.Field
								><Field.FieldLabel for="reason">Lý do sửa</Field.FieldLabel><Input
									id="reason"
									bind:value={amendmentReason}
									required
									disabled={match?.status !== 'completed'}
								/></Field.Field
							><Button
								type="submit"
								variant="outline"
								disabled={saving || match?.status !== 'completed' || !amendmentReason.trim()}
								>Lưu bản sửa kết quả</Button
							></Field.FieldGroup
						>
					</form></Card.Content
				></Card.Root
			>
			<Card.Root
				><Card.Header
					><Card.Title>Dòng sự kiện</Card.Title><Card.Description
						>{events.length} thay đổi đã lưu</Card.Description
					></Card.Header
				><Card.Content class="max-h-[50svh] overflow-auto p-0"
					><Table.Root
						><Table.Body
							>{#each events as event, i (event.sequence)}<Table.Row
									class={i === index ? 'bg-muted' : ''}
									><Table.Cell
										><button
											class="flex w-full cursor-pointer flex-col gap-1 text-left"
											onclick={() => {
												stop();
												index = i;
											}}
											><span class="font-medium">{event.type}</span><span
												class="text-xs text-muted-foreground"
												>#{event.sequence} · {new Date(event.created_at).toLocaleTimeString(
													'vi-VN'
												)}</span
											></button
										></Table.Cell
									></Table.Row
								>{/each}</Table.Body
						></Table.Root
					></Card.Content
				></Card.Root
			>
		</div>
	</div>
{/if}
