<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { socket } from '$lib/socket';
	import type { Match, MatchInput, MatchStatus, QuestionSet } from '$lib/types';
	import * as Alert from '$lib/components/ui/alert';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as Card from '$lib/components/ui/card';
	import * as Field from '$lib/components/ui/field';
	import * as Popover from '$lib/components/ui/popover';
	import * as Table from '$lib/components/ui/table';
	import * as Select from '$lib/components/ui/select';
	import { Badge } from '$lib/components/ui/badge';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Spinner } from '$lib/components/ui/spinner';
	import Copy from '@lucide/svelte/icons/copy';
	import History from '@lucide/svelte/icons/history';
	import Pencil from '@lucide/svelte/icons/pencil';
	import Plus from '@lucide/svelte/icons/plus';
	import Search from '@lucide/svelte/icons/search';
	import ScrollText from '@lucide/svelte/icons/scroll-text';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Play from '@lucide/svelte/icons/play';
	import Pause from '@lucide/svelte/icons/pause';

	const statusLabels: Record<MatchStatus, string> = {
		draft: 'Bản nháp',
		ready: 'Sẵn sàng',
		playing: 'Đang thi',
		paused: 'Tạm dừng',
		completed: 'Hoàn thành',
		archived: 'Đã lưu trữ'
	};
	let matches = $state<Match[]>([]),
		questionSets = $state<QuestionSet[]>([]),
		search = $state(''),
		loading = $state(true),
		busy = $state(false),
		error = $state('');
	let editorOpen = $state(false),
		editing = $state<Match | null>(null),
		switchOpen = $state(false),
		switching = $state<Match | null>(null),
		countdown = $state(5),
		countdownTimer: ReturnType<typeof setInterval> | null = null;
	let form = $state<MatchInput>(emptyInput());
	let runtime = $derived(socket.match.current);
	let selected = $derived(matches.find((value) => value.selected) ?? null);

	onMount(() => {
		void load();
	});
	async function load() {
		loading = true;
		error = '';
		try {
			[matches, questionSets] = await Promise.all([
				api.matches.list(search),
				api.questionSets.list()
			]);
		} catch (e) {
			error = message(e);
		} finally {
			loading = false;
		}
	}
	function emptyInput(): MatchInput {
		return {
			name: '',
			season: 9,
			stage: 'Tứ kết',
			question_set_id: '',
			contestants: Array.from({ length: 4 }, (_, i) => ({
				slot: i + 1,
				user_id: 0,
				name: '',
				class: ''
			}))
		};
	}
	function openCreate() {
		editing = null;
		form = emptyInput();
		editorOpen = true;
	}
	function openEdit(value: Match) {
		editing = value;
		form = {
			name: value.name,
			season: value.season,
			stage: value.stage,
			question_set_id: value.question_set_id ?? '',
			version: value.version,
			contestants: value.contestants.map((c) => ({ ...c }))
		};
		editorOpen = true;
	}
	async function save(event: SubmitEvent) {
		event.preventDefault();
		busy = true;
		error = '';
		try {
			if (editing) await api.matches.update(editing.id, form);
			else await api.matches.create(form);
			editorOpen = false;
			await load();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	async function duplicate(value: Match) {
		busy = true;
		try {
			await api.matches.duplicate(value.id);
			await load();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	async function archive(value: Match) {
		busy = true;
		try {
			await api.matches.archive(value);
			await load();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	function askSwitch(value: Match) {
		switching = value;
		countdown = 5;
		switchOpen = true;
		if (countdownTimer) clearInterval(countdownTimer);
		countdownTimer = setInterval(() => {
			countdown = Math.max(0, countdown - 1);
			if (countdown === 0 && countdownTimer) {
				clearInterval(countdownTimer);
				countdownTimer = null;
			}
		}, 1000);
	}
	async function confirmSwitch() {
		if (!switching || countdown > 0) return;
		busy = true;
		try {
			await socket.match.select(switching.id);
			switchOpen = false;
			await load();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	async function lifecycle(operation: 'ready' | 'start' | 'pause' | 'resume' | 'complete') {
		busy = true;
		try {
			await socket.match[operation]();
			await load();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	async function unload() {
		busy = true;
		try {
			await socket.match.unload();
			await load();
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
		}
	}
	function badgeVariant(status: MatchStatus): 'default' | 'secondary' | 'outline' | 'destructive' {
		if (status === 'playing') return 'default';
		if (status === 'paused') return 'destructive';
		if (status === 'completed') return 'secondary';
		return 'outline';
	}
	function message(value: unknown) {
		return value instanceof Error ? value.message : 'Không thể hoàn thành thao tác';
	}
</script>

<svelte:head><title>Trận đấu - {brand.appName}</title></svelte:head>
<header class="flex items-center justify-between gap-4">
	<h1 class="text-3xl font-bold tracking-tight">Trận đấu</h1>
	<Button onclick={openCreate}><Plus data-icon="inline-start" />Thêm trận đấu</Button>
</header>
{#if error}<Alert.Root variant="destructive"
		><Alert.Title>Thao tác thất bại</Alert.Title><Alert.Description>{error}</Alert.Description
		></Alert.Root
	>{/if}
<Card.Root>
	<Card.Header><Card.Title>Trận đang được chọn</Card.Title></Card.Header>
	<Card.Content>
		{#if selected}<div class="grid gap-5 lg:grid-cols-[1fr_auto]">
				<div class="flex flex-col gap-2">
					<div class="flex flex-wrap items-center gap-2">
						<h2 class="text-xl font-semibold">{selected.name}</h2>
						<Badge variant={badgeVariant(runtime?.status ?? selected.status)}
							>{statusLabels[runtime?.status ?? selected.status]}</Badge
						>
					</div>
					<p class="text-sm text-muted-foreground">
						Mùa {selected.season} · {selected.stage} · phiên {runtime?.epoch ?? '—'} / bản {runtime?.version ??
							selected.version}
					</p>
					<div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
						{#each selected.contestants as contestant (contestant.slot)}<div
								class="rounded-md border p-3"
							>
								<p class="font-medium">{contestant.name}</p>
								<p class="text-sm text-muted-foreground">
									Vị trí {contestant.slot} · {contestant.class || 'Chưa có lớp'}
								</p>
							</div>{/each}
					</div>
				</div>
				<div class="flex flex-wrap content-start gap-2 lg:max-w-72 lg:justify-end">
					<Button variant="outline" href={`#/match/${selected.id}/logs`}
						><ScrollText data-icon="inline-start" />Xem nhật ký</Button
					>
					{#if (runtime?.status ?? selected.status) === 'draft'}<Button
							onclick={() => lifecycle('ready')}
							disabled={busy}>Sẵn sàng</Button
						>{:else if (runtime?.status ?? selected.status) === 'ready'}<Button
							onclick={() => lifecycle('start')}
							disabled={busy}>Bắt đầu</Button
						>{:else if runtime?.status === 'playing'}<Button
							variant="destructive"
							onclick={() => lifecycle('pause')}
							disabled={busy}><Pause />Tạm dừng</Button
						>{:else if runtime?.status === 'paused'}<Button
							onclick={() => lifecycle('resume')}
							disabled={busy}><Play />Tiếp tục</Button
						><Button variant="outline" onclick={() => lifecycle('complete')} disabled={busy}
							>Hoàn thành</Button
						>{/if}
					<Popover.Root>
						<Popover.Trigger
							class={buttonVariants({ variant: 'outline' })}
							disabled={busy || runtime?.status === 'playing'}>Bỏ chọn trận</Popover.Trigger
						>
						<Popover.Content align="end" class="w-72">
							<Popover.Title>Bỏ tải trận đang chọn?</Popover.Title>
							<Popover.Description
								>Trạng thái hiện tại và sự kiện dỡ trận sẽ được lưu.</Popover.Description
							>
							<div class="flex justify-end gap-2">
								<Popover.Close class={buttonVariants({ variant: 'outline', size: 'sm' })}
									>Hủy</Popover.Close
								>
								<Popover.Close class={buttonVariants({ size: 'sm' })} onclick={unload}
									>Bỏ chọn</Popover.Close
								>
							</div>
						</Popover.Content>
					</Popover.Root>
				</div>
			</div>
		{:else}<Alert.Root
				><Alert.Title>Chưa chọn trận đấu</Alert.Title><Alert.Description
					>Tạo hoặc chọn một trận bên dưới trước khi mở bảng điều khiển.</Alert.Description
				></Alert.Root
			>{/if}
	</Card.Content>
</Card.Root>

<section class="flex flex-col gap-4">
	<div class="flex flex-col gap-3 sm:flex-row">
		<div class="relative flex-1">
			<Search
				class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
			/><Input
				class="pl-10"
				bind:value={search}
				placeholder="Tìm theo tên hoặc vòng thi"
				onkeydown={(e) => e.key === 'Enter' && load()}
			/>
		</div>
		<Button variant="outline" onclick={load} disabled={loading}>Tìm kiếm</Button>
	</div>
	<div class="overflow-hidden rounded-lg border">
		<Table.Root
			><Table.Header
				><Table.Row
					><Table.Head>Trận đấu</Table.Head><Table.Head>Vòng</Table.Head><Table.Head
						>Đề thi</Table.Head
					><Table.Head>Trạng thái</Table.Head><Table.Head class="text-right">Thao tác</Table.Head
					></Table.Row
				></Table.Header
			><Table.Body>
				{#if loading}<Table.Row
						><Table.Cell colspan={5} class="h-28 text-center"
							><Spinner class="mx-auto" /><span class="sr-only">Đang tải</span></Table.Cell
						></Table.Row
					>{:else if matches.length === 0}<Table.Row
						><Table.Cell colspan={5} class="h-28 text-center text-muted-foreground"
							>Chưa có trận đấu phù hợp.</Table.Cell
						></Table.Row
					>{:else}{#each matches as value (value.id)}<Table.Row
							><Table.Cell
								><div class="flex flex-col gap-1">
									<span class="font-medium">{value.name}</span><span
										class="text-xs text-muted-foreground">Mùa {value.season}</span
									>
								</div></Table.Cell
							><Table.Cell>{value.stage}</Table.Cell><Table.Cell
								>{questionSets.find((q) => q.id === value.question_set_id)?.name ??
									'Chưa gán'}</Table.Cell
							><Table.Cell
								><Badge variant={badgeVariant(value.status)}>{statusLabels[value.status]}</Badge
								>{#if value.selected}<Badge class="ml-2" variant="outline">Đang chọn</Badge
									>{/if}</Table.Cell
							><Table.Cell
								><div class="flex flex-wrap justify-end gap-2">
									{#if !value.selected}<Button
											size="sm"
											onclick={() => askSwitch(value)}
											disabled={runtime?.status === 'playing' ||
												busy ||
												value.status === 'archived'}>Chuyển sang trận này</Button
										>{/if}<Button
										size="icon-sm"
										variant="outline"
										aria-label="Sửa trận"
										onclick={() => openEdit(value)}
										disabled={value.status === 'completed' || value.status === 'archived'}
										><Pencil /></Button
									><Button
										size="icon-sm"
										variant="outline"
										aria-label="Nhân bản trận"
										onclick={() => duplicate(value)}><Copy /></Button
									><Button
										size="icon-sm"
										variant="outline"
										aria-label="Xem phát lại"
										href={`#/match/${value.id}/replay`}><History /></Button
									>{#if value.selected || value.status === 'archived'}<Button
											size="icon-sm"
											variant="destructive"
											aria-label="Không thể lưu trữ trận"
											disabled><Trash2 /></Button
										>{:else}<Popover.Root>
											<Popover.Trigger
												class={buttonVariants({ variant: 'destructive', size: 'icon-sm' })}
												aria-label="Lưu trữ trận"><Trash2 /></Popover.Trigger
											>
											<Popover.Content align="end" class="w-auto"
												><Popover.Title>Lưu trữ trận đấu này?</Popover.Title>
												<div class="flex justify-end gap-2">
													<Popover.Close class={buttonVariants({ variant: 'outline', size: 'sm' })}
														>Hủy</Popover.Close
													><Popover.Close
														class={buttonVariants({ variant: 'destructive', size: 'sm' })}
														onclick={() => archive(value)}>Lưu trữ</Popover.Close
													>
												</div></Popover.Content
											>
										</Popover.Root>{/if}
								</div></Table.Cell
							></Table.Row
						>{/each}{/if}
			</Table.Body></Table.Root
		>
	</div>
</section>

{#if editorOpen}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		role="presentation"
	>
		<div
			class="max-h-[90svh] w-full max-w-3xl overflow-y-auto rounded-lg border bg-background p-6 shadow-xl"
			role="dialog"
			aria-modal="true"
			aria-labelledby="match-editor-title"
		>
			<header class="mb-5">
				<h2 id="match-editor-title" class="text-lg font-semibold">
					{editing ? 'Sửa trận đấu' : 'Thêm trận đấu'}
				</h2>
				<p class="text-sm text-muted-foreground">
					Bốn tài khoản thí sinh được gắn cố định theo vị trí.
				</p>
			</header>
			<form onsubmit={save}>
				<Field.FieldGroup>
					<div class="grid gap-5 md:grid-cols-2">
						<Field.Field
							><Field.FieldLabel for="match-name">Tên trận</Field.FieldLabel><Input
								id="match-name"
								bind:value={form.name}
								required
								maxlength={120}
							/></Field.Field
						>
						<Field.Field
							><Field.FieldLabel for="stage">Vòng thi</Field.FieldLabel><Input
								id="stage"
								bind:value={form.stage}
								list="stage-options"
								required
							/><datalist id="stage-options"
								><option value="Tứ kết"></option><option value="Bán kết"></option><option
									value="Chung kết"
								></option><option value="Vòng bảng"></option></datalist
							></Field.Field
						>
						<Field.Field
							><Field.FieldLabel for="season">Mùa</Field.FieldLabel><Input
								id="season"
								type="number"
								bind:value={form.season}
								min={1}
								max={99}
								required
							/></Field.Field
						>
					</div>
					<Field.Field
						><Field.FieldLabel for="question-set">Đề thi</Field.FieldLabel><Select.Root
							type="single"
							bind:value={form.question_set_id}
							><Select.Trigger id="question-set"
								>{questionSets.find((questionSet) => questionSet.id === form.question_set_id)
									?.name ?? 'Chọn đề thi'}</Select.Trigger
							><Select.Content
								><Select.Group
									>{#each questionSets.filter((questionSet) => !questionSet.archived_at) as questionSet (questionSet.id)}<Select.Item
											value={questionSet.id}
											>{questionSet.name} · bản {questionSet.revision}</Select.Item
										>{/each}</Select.Group
								></Select.Content
							></Select.Root
						><Field.FieldDescription>Để trống nếu đây là bản nháp.</Field.FieldDescription
						></Field.Field
					>
					<Field.FieldSet
						><Field.FieldLegend>Bốn thí sinh</Field.FieldLegend><Field.FieldGroup
							>{#each form.contestants as contestant, index (contestant.slot)}<div
									class="grid gap-4 rounded-md border p-4 md:grid-cols-[auto_1fr_1fr]"
								>
									<Badge variant="outline">Vị trí {index + 1}</Badge><Field.Field
										><Field.FieldLabel for={`name-${index}`}>Họ tên</Field.FieldLabel><Input
											id={`name-${index}`}
											bind:value={contestant.name}
											required
										/></Field.Field
									><Field.Field
										><Field.FieldLabel for={`class-${index}`}>Lớp</Field.FieldLabel><Input
											id={`class-${index}`}
											bind:value={contestant.class}
										/></Field.Field
									>
								</div>{/each}</Field.FieldGroup
						></Field.FieldSet
					>
					<footer class="flex justify-end gap-2">
						<Button type="button" variant="outline" onclick={() => (editorOpen = false)}>Hủy</Button
						><Button type="submit" disabled={busy}
							>{#if busy}<Spinner data-icon="inline-start" />{/if}Lưu trận đấu</Button
						>
					</footer>
				</Field.FieldGroup>
			</form>
		</div>
	</div>
{/if}

<AlertDialog.Root bind:open={switchOpen}
	><AlertDialog.Content
		><AlertDialog.Header
			><AlertDialog.Title>Chuyển trận đang hoạt động?</AlertDialog.Title><AlertDialog.Description
				>Trạng thái hiện tại sẽ được lưu trước khi chuyển sang “{switching?.name}”.</AlertDialog.Description
			></AlertDialog.Header
		><AlertDialog.Footer
			><AlertDialog.Cancel>Hủy</AlertDialog.Cancel><AlertDialog.Action
				onclick={confirmSwitch}
				disabled={countdown > 0 || busy}
				>Chuyển trận {countdown > 0 ? `(${countdown})` : ''}</AlertDialog.Action
			></AlertDialog.Footer
		></AlertDialog.Content
	></AlertDialog.Root
>
