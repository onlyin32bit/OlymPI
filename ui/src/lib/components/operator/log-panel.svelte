<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { api } from '$lib/api';
	import { Badge } from '$lib/components/ui/badge';
	import { Spinner } from '$lib/components/ui/spinner';
	import { buildLogEntries, groupLogEntries, type LogEntry, type LogView } from '$lib/match-logs';
	import { socket } from '$lib/socket';
	import type { MatchEvent } from '$lib/types';

	const views = [
		{ id: 'match', label: 'Diễn biến' },
		{ id: 'operations', label: 'Thao tác' },
		{ id: 'technical', label: 'Kỹ thuật' }
	] as const;
	let matchEvents = $state<MatchEvent[]>([]);
	let loading = $state(true);
	let error = $state('');
	let view = $state<LogView>('match');
	let phaseFilter = $state('');
	let questionFilter = $state('');
	let slotFilter = $state('');
	let following = $state(true);
	let unread = $state(0);
	let rawExpanded = $state<Record<string, boolean>>({});
	let expandedGroups = $state<Record<string, boolean>>({});
	let logView = $state<HTMLDivElement>();
	let mounted = false;
	let loadedMatchID = '';
	let generation = 0;
	let lastRuntimeKey = '';
	let request: AbortController | null = null;
	let reloadQueued = false;
	let refreshTimer: ReturnType<typeof setTimeout> | undefined;

	let entries = $derived(buildLogEntries(matchEvents));
	let phaseOptions = $derived([...new Map(entries.filter((entry) => entry.phase).map((entry) =>
		[entry.phase, { value: entry.phase, label: entry.phaseLabel }]
	)).values()]);
	let questionOptions = $derived([...new Map(entries.filter((entry) => entry.questionID && (!phaseFilter || entry.phase === phaseFilter)).map((entry) =>
		[entry.questionID, { value: entry.questionID, label: phaseFilter ? entry.questionLabel : `${entry.phaseLabel} · ${entry.questionLabel}` }]
	)).values()]);
	let filtered = $derived(entries.filter((entry) =>
		(!phaseFilter || entry.phase === phaseFilter) &&
		(!questionFilter || entry.questionID === questionFilter) &&
		(!slotFilter || entry.slots.includes(Number(slotFilter)))
	));
	let visible = $derived(filtered.filter((entry) => view === 'technical' || entry.views.includes(view)));
	let groups = $derived(groupLogEntries(filtered).filter((group) => group.entries.some((entry) => entry.views.includes(view === 'operations' ? 'operations' : 'match'))));
	let latestGroupID = $derived(groups.at(-1)?.id);
	let hasFilters = $derived(Boolean(phaseFilter || questionFilter || slotFilter));

	onMount(() => {
		mounted = true;
		const unsubscribe = socket.match.onUpdate((runtime) => {
			if (!mounted) return;
			const matchID = runtime?.match_id ?? '';
			const key = runtime ? `${matchID}:${runtime.epoch}:${runtime.version}` : '';
			if (matchID !== loadedMatchID) {
				generation++;
				request?.abort();
				request = null;
				clearTimeout(refreshTimer);
				refreshTimer = undefined;
				reloadQueued = false;
				loadedMatchID = matchID;
				matchEvents = [];
				phaseFilter = questionFilter = slotFilter = '';
				rawExpanded = {};
				expandedGroups = {};
				following = true;
				unread = 0;
				error = '';
				loading = Boolean(matchID);
				lastRuntimeKey = key;
				if (matchID) queueLoad(true);
				return;
			}
			if (!matchID) { loading = false; return; }
			if (key === lastRuntimeKey && !error) return;
			lastRuntimeKey = key;
			queueLoad();
		});
		return () => {
			mounted = false;
			generation++;
			unsubscribe();
			clearTimeout(refreshTimer);
			request?.abort();
		};
	});

	function queueLoad(immediate = false): void {
		if (!mounted || refreshTimer !== undefined) return;
		if (request) { reloadQueued = true; return; }
		refreshTimer = setTimeout(() => {
			refreshTimer = undefined;
			void load();
		}, immediate ? 0 : 200);
	}

	async function load(): Promise<void> {
		if (!mounted || !loadedMatchID) return;
		if (request) { reloadQueued = true; return; }
		const mine = new AbortController();
		request = mine;
		const matchID = loadedMatchID;
		const requestGeneration = generation;
		try {
			const cursor = matchEvents.at(-1)?.sequence ?? 0;
			const incoming = await api.matches.events(matchID, { afterSequence: cursor, signal: mine.signal });
			if (!mounted || generation !== requestGeneration || loadedMatchID !== matchID) return;
			const follow = following && nearBottom();
			const knownVisible = new Set(visible.map((entry) => entry.id));
			const additions = incoming.filter((entry) => entry.match_id === matchID && entry.sequence > cursor);
			const merged = new Map([...matchEvents, ...additions].map((entry) => [entry.sequence, entry]));
			matchEvents = [...merged.values()].sort((a, b) => a.sequence - b.sequence);
			loading = false;
			error = '';
			await tick();
			if (!mounted || generation !== requestGeneration) return;
			if (follow) jumpToLatest();
			else unread += visible.filter((entry) => !knownVisible.has(entry.id)).length;
		} catch (cause) {
			if (mounted && generation === requestGeneration && !mine.signal.aborted) {
				error = cause instanceof Error ? cause.message : 'Không thể tải nhật ký trận';
				loading = false;
			}
		} finally {
			if (request === mine) {
				request = null;
				if (reloadQueued) { reloadQueued = false; queueLoad(); }
			}
		}
	}

	function nearBottom(): boolean {
		return !logView || logView.scrollHeight - logView.scrollTop - logView.clientHeight <= 32;
	}
	function onScroll(): void {
		following = nearBottom();
		if (following) unread = 0;
	}
	function jumpToLatest(): void {
		logView?.scrollTo({ top: logView.scrollHeight, behavior: 'instant' });
		following = true;
		unread = 0;
	}
	function resetView(): void {
		following = true;
		unread = 0;
		void tick().then(() => { if (mounted) jumpToLatest(); });
	}
	function time(value: string): string {
		return new Date(value).toLocaleTimeString('vi-VN', { hour12: false });
	}
	function inspect(event: Event): void {
		if ((event.currentTarget as HTMLDetailsElement).open) following = false;
	}
</script>

{#snippet row(entry: LogEntry, technical = false)}
	<li class="flex gap-2 border-b border-border px-3 py-2 last:border-b-0" class:important={entry.important} data-log-sequence={entry.sequence}>
		<time class="w-16 shrink-0 pt-0.5 text-xs text-muted-foreground tabular-nums" datetime={entry.event.created_at}>{time(entry.event.created_at)}</time>
		<div class="min-w-0 flex-1">
			<p class="text-[13px] leading-5 font-medium break-words text-foreground">{technical ? entry.event.type : entry.title}</p>
			{#if !technical && entry.detail}
				{@const lines = entry.detail.split('\n')}
				<p class="mt-0.5 text-xs leading-5 break-words text-muted-foreground">{lines[0]}</p>
				{#if lines.length > 1}
					<details class="mt-1 text-xs" ontoggle={inspect}>
						<summary class="w-fit cursor-pointer text-muted-foreground">Chi tiết · {lines.length - 1} mục khác</summary>
						<p class="mt-1 leading-5 break-words whitespace-pre-line text-muted-foreground">{lines.slice(1).join('\n')}</p>
					</details>
				{/if}
			{/if}
			{#if !technical && entry.answer !== undefined}
				<details class="mt-1 text-xs" ontoggle={inspect}>
					<summary class="w-fit cursor-pointer text-foreground underline-offset-2 hover:underline">Xem câu trả lời</summary>
					<p class="mt-1 rounded-sm bg-muted p-2 break-words whitespace-pre-wrap">{entry.answer || '—'}</p>
				</details>
			{/if}
			{#if technical}
				<details class="mt-1 text-xs" ontoggle={(event) => { inspect(event); rawExpanded[entry.id] = event.currentTarget.open; }}>
					<summary class="w-fit cursor-pointer text-muted-foreground">Dữ liệu sự kiện · #{entry.sequence}</summary>
					{#if rawExpanded[entry.id]}
						<pre class="mt-1 max-h-64 overflow-auto rounded-sm bg-muted p-2 text-[11px] leading-5">{JSON.stringify({ actor_id: entry.event.actor_id, client_id: entry.event.client_id, payload: entry.event.payload, state: entry.event.state }, null, 2)}</pre>
					{/if}
				</details>
			{/if}
		</div>
	</li>
{/snippet}

<section class="flex h-full min-h-0 flex-col overflow-hidden rounded-md bg-card" aria-label="Nhật ký trận">
	<header class="flex shrink-0 items-center justify-between gap-3 px-3 pt-2 pb-1">
		<h2 class="truncate text-sm font-semibold" title={socket.match.current?.match_name}>Nhật ký trận{socket.match.current?.match_name ? ` · ${socket.match.current.match_name}` : ''}</h2>
		{#if socket.match.current}<Badge variant="outline" class="shrink-0 text-[10px]">{visible.length} {view === 'technical' ? 'sự kiện' : 'mục'}</Badge>{/if}
	</header>
	<div class="flex shrink-0 border-b px-2" role="group" aria-label="Loại nhật ký">
		{#each views as item (item.id)}
			<button type="button" aria-pressed={view === item.id} onclick={() => { view = item.id; resetView(); }} class="h-8 flex-1 border-b-2 px-2 text-xs font-medium focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring" class:border-foreground={view === item.id} class:text-foreground={view === item.id} class:border-transparent={view !== item.id} class:text-muted-foreground={view !== item.id}>{item.label}</button>
		{/each}
	</div>
	<div class="flex shrink-0 gap-1.5 border-b px-2 py-1.5">
		<select class="h-7 min-w-0 flex-1 rounded-sm border border-border bg-card px-1.5 text-xs" aria-label="Lọc theo vòng" bind:value={phaseFilter} onchange={() => { questionFilter = ''; resetView(); }}>
			<option value="">Tất cả vòng</option>
			{#each phaseOptions as option (option.value)}<option value={option.value}>{option.label}</option>{/each}
		</select>
		<select class="h-7 min-w-0 flex-1 rounded-sm border border-border bg-card px-1.5 text-xs" aria-label="Lọc theo câu hỏi" bind:value={questionFilter} onchange={resetView}>
			<option value="">Tất cả câu</option>
			{#each questionOptions as option (option.value)}<option value={option.value}>{option.label}</option>{/each}
		</select>
		<select class="h-7 min-w-0 flex-1 rounded-sm border border-border bg-card px-1.5 text-xs" aria-label="Lọc theo thí sinh" bind:value={slotFilter} onchange={resetView}>
			<option value="">Tất cả thí sinh</option>
			{#each socket.match.current?.contestants ?? [] as contestant (contestant.slot)}<option value={String(contestant.slot)}>{contestant.name || `Thí sinh ${contestant.slot}`}</option>{/each}
		</select>
		{#if hasFilters}<button type="button" class="shrink-0 px-1 text-xs text-foreground underline-offset-2 hover:underline" onclick={() => { phaseFilter = questionFilter = slotFilter = ''; resetView(); }}>Xóa lọc</button>{/if}
	</div>
	{#if error}<div class="flex items-center justify-between gap-2 border-b px-3 py-2 text-xs text-destructive" role="alert"><p>{error}</p><button type="button" class="shrink-0 underline" onclick={() => void load()}>Tải lại</button></div>{/if}
	<div bind:this={logView} class="min-h-0 flex-1 overflow-y-auto overscroll-contain" onscroll={onScroll} aria-label="Nội dung nhật ký">
		{#if loading}
			<div class="flex h-full items-center justify-center"><Spinner /></div>
		{:else if !socket.match.current}
			<p class="p-6 text-center text-sm text-muted-foreground">Chưa tải trận đấu.</p>
		{:else if visible.length === 0}
			<p class="p-6 text-center text-xs leading-5 text-muted-foreground">{hasFilters ? 'Không có sự kiện khớp bộ lọc.' : view === 'match' ? 'Chưa có diễn biến thi đấu. Thao tác điều khiển nằm trong “Thao tác”.' : 'Chưa có sự kiện trong mục này.'}</p>
		{:else if view === 'technical'}
			<ul>{#each visible as entry (entry.id)}{@render row(entry, true)}{/each}</ul>
		{:else}
			{#each groups as group (group.id)}
				<details open={expandedGroups[group.id] ?? group.id === latestGroupID} class="border-b border-border last:border-b-0" ontoggle={(event) => { expandedGroups[group.id] = event.currentTarget.open; }}>
					<summary class="cursor-pointer bg-muted px-3 py-2 text-xs font-semibold text-muted-foreground" onclick={() => { if (group.id !== latestGroupID) following = false; }}>{group.label}<span class="ml-2 font-normal text-muted-foreground">{group.entries.filter((entry) => entry.views.includes(view === 'operations' ? 'operations' : 'match')).length} mục</span></summary>
					<ul>{#each group.entries.filter((entry) => entry.views.includes(view === 'operations' ? 'operations' : 'match')) as entry (entry.id)}{@render row(entry)}{/each}</ul>
					{#if view === 'match'}
						{@const operations = group.entries.filter((entry) => entry.views.includes('operations') && !entry.views.includes('match'))}
						{#if operations.length}<details class="px-3 py-2 text-xs" ontoggle={inspect}><summary class="cursor-pointer text-muted-foreground">{operations.length} thao tác điều khiển</summary><ul class="mt-1">{#each operations as entry (entry.id)}{@render row(entry)}{/each}</ul></details>{/if}
					{/if}
				</details>
			{/each}
		{/if}
	</div>
	{#if !following}
		<div class="flex shrink-0 justify-center border-t bg-card px-2 py-1.5" aria-live="polite"><button type="button" class="rounded-sm px-3 py-1 text-xs font-medium text-foreground underline-offset-2 hover:underline hover:bg-muted" onclick={jumpToLatest}>{unread ? `${unread} sự kiện mới · Xem mới nhất` : 'Theo dõi mới nhất'}</button></div>
	{/if}
</section>

<style>
	.important { border-left: 2px solid #f59e0b; background: color-mix(in oklab, #f59e0b 12%, transparent); }
</style>
