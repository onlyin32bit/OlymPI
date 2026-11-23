<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import type { QuestionSet } from '$lib/types';
	import * as Alert from '$lib/components/ui/alert';
	import * as Popover from '$lib/components/ui/popover';
	import * as Table from '$lib/components/ui/table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Spinner } from '$lib/components/ui/spinner';
	import Copy from '@lucide/svelte/icons/copy';
	import Download from '@lucide/svelte/icons/download';
	import Pencil from '@lucide/svelte/icons/pencil';
	import Plus from '@lucide/svelte/icons/plus';
	import Search from '@lucide/svelte/icons/search';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Upload from '@lucide/svelte/icons/upload';

	let values = $state<QuestionSet[]>([]),
		search = $state(''),
		loading = $state(true),
		error = $state(''),
		notice = $state(''),
		importing = $state(false),
		exporting = $state('');
	let importInput = $state<HTMLInputElement | null>(null);
	onMount(() => {
		void load();
	});
	async function load() {
		loading = true;
		error = '';
		try {
			values = await api.questionSets.list(search);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Không thể tải đề thi';
		} finally {
			loading = false;
		}
	}
	async function archive(value: QuestionSet) {
		try {
			await api.questionSets.archive(value);
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Không thể lưu trữ đề thi';
		}
	}
	async function importArchive(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		importing = true;
		error = '';
		notice = '';
		try {
			const created = await api.questionSets.importArchive(file);
			notice = `Đã nhập “${created.name}”.`;
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Không thể nhập đề thi';
		} finally {
			importing = false;
			input.value = '';
		}
	}
	async function exportArchive(value: QuestionSet) {
		exporting = value.id;
		error = '';
		try {
			const { blob, filename } = await api.questionSets.exportArchive(value.id);
			const url = URL.createObjectURL(blob);
			const link = document.createElement('a');
			link.href = url;
			link.download = filename;
			link.click();
			URL.revokeObjectURL(url);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Không thể xuất đề thi';
		} finally {
			exporting = '';
		}
	}
</script>

<svelte:head><title>Đề thi - {brand.appName}</title></svelte:head>

<header class="flex items-center justify-between gap-4">
	<h1 class="text-3xl font-bold tracking-tight">Đề thi</h1>
	<div class="flex gap-2">
		<Input
			bind:ref={importInput}
			class="hidden"
			type="file"
			accept=".zip,application/zip"
			onchange={importArchive}
		/>
		<Button variant="outline" disabled={importing} onclick={() => importInput?.click()}
			>{#if importing}<Spinner data-icon="inline-start" />{:else}<Upload
					data-icon="inline-start"
				/>{/if}Nhập ZIP</Button
		>
		<Button href="#/question-set/new"><Plus data-icon="inline-start" />Thêm đề thi</Button>
	</div>
</header>
{#if error}<Alert.Root variant="destructive"
		><Alert.Title>Không thể hoàn thành</Alert.Title><Alert.Description>{error}</Alert.Description
		></Alert.Root
	>{:else if notice}<Alert.Root><Alert.Title>{notice}</Alert.Title></Alert.Root>{/if}
<div class="flex flex-col gap-3 sm:flex-row">
	<div class="relative flex-1">
		<Search
			class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
		/><Input
			class="pl-10"
			bind:value={search}
			placeholder="Tìm tên hoặc danh mục"
			onkeydown={(e) => e.key === 'Enter' && load()}
		/>
	</div>
	<Button variant="outline" onclick={load}>Tìm kiếm</Button>
</div>
<div class="overflow-hidden rounded-lg border">
	<Table.Root
		><Table.Header
			><Table.Row
				><Table.Head>Tên đề</Table.Head><Table.Head>Danh mục</Table.Head><Table.Head
					>Phiên bản</Table.Head
				><Table.Head>Trận liên kết</Table.Head><Table.Head class="text-right">Thao tác</Table.Head
				></Table.Row
			></Table.Header
		><Table.Body>
			{#if loading}<Table.Row
					><Table.Cell colspan={5} class="h-28 text-center"><Spinner class="mx-auto" /></Table.Cell
					></Table.Row
				>{:else if values.length === 0}<Table.Row
					><Table.Cell colspan={5} class="h-28 text-center text-muted-foreground"
						>Chưa có đề thi phù hợp.</Table.Cell
					></Table.Row
				>{:else}{#each values as value (value.id)}<Table.Row
						><Table.Cell
							><div class="flex items-center gap-2">
								<span class="font-medium">{value.name}</span>{#if value.archived_at}<Badge
										variant="secondary">Đã lưu trữ</Badge
									>{/if}
							</div></Table.Cell
						><Table.Cell>{value.category || '—'}</Table.Cell><Table.Cell
							><Badge variant="outline">Bản {value.revision}</Badge></Table.Cell
						><Table.Cell>{value.linked_matches}</Table.Cell><Table.Cell
							><div class="flex justify-end gap-2">
								<Button
									size="icon"
									variant="outline"
									href={`#/question-set/${value.id}`}
									aria-label="Sửa đề thi"><Pencil /></Button
								><Button
									size="icon"
									variant="outline"
									href={`#/question-set/${value.id}?duplicate=1`}
									aria-label="Nhân bản đề thi"><Copy /></Button
								><Button
									size="icon"
									variant="outline"
									disabled={exporting === value.id}
									onclick={() => exportArchive(value)}
									aria-label="Xuất đề thi"
									>{#if exporting === value.id}<Spinner />{:else}<Download />{/if}</Button
								>{#if value.archived_at}<Button
										size="icon"
										variant="destructive"
										disabled
										aria-label="Đề thi đã lưu trữ"><Trash2 /></Button
									>{:else}<Popover.Root>
										<Popover.Trigger
											class={buttonVariants({ variant: 'destructive', size: 'icon' })}
											aria-label="Lưu trữ đề thi"><Trash2 /></Popover.Trigger
										>
										<Popover.Content align="end" class="w-auto"
											><Popover.Title>Lưu trữ đề thi này?</Popover.Title>
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
