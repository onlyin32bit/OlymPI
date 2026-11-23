<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import type { Asset, Question, QuestionSetData, QuestionType } from '$lib/types';
	import { RichText } from '$lib/components/common';
	import * as Alert from '$lib/components/ui/alert';
	import * as Field from '$lib/components/ui/field';
	import * as Popover from '$lib/components/ui/popover';
	import * as Select from '$lib/components/ui/select';
	import * as Tabs from '$lib/components/ui/tabs';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Spinner } from '$lib/components/ui/spinner';
	import { Textarea } from '$lib/components/ui/textarea';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import Plus from '@lucide/svelte/icons/plus';
	import Save from '@lucide/svelte/icons/save';
	import Trash2 from '@lucide/svelte/icons/trash-2';

	type EditorOptions = {
		types: QuestionType[];
		showTime?: boolean;
		explanation?: boolean;
		label?: string;
	};
	const standardTypes: QuestionType[] = ['text', 'image', 'audio'];
	const finishTypes: QuestionType[] = ['text', 'image', 'audio', 'video'];
	const speedTypes: QuestionType[] = ['image', 'video'];
	const typeLabels: Record<QuestionType, string> = {
		text: 'Văn bản',
		image: 'Hình ảnh',
		audio: 'Âm thanh',
		video: 'Video'
	};
	const id = page.params.id ?? 'new';
	const isNew = id === 'new';
	let name = $state('');
	let category = $state('');
	let version = $state(0);
	let data = $state<QuestionSetData>(defaults());
	let activeSection = $state('kd_chung');
	let kdSlot = $state('0');
	let vdSlot = $state('0');
	let loading = $state(!isNew);
	let saving = $state(false);
	let uploads = $state(0);
	let error = $state('');

	onMount(() => {
		void load();
	});

	function question(type: QuestionType = 'text'): Question {
		return { content: '', answer: '', type };
	}

	function defaults(): QuestionSetData {
		return {
			kd_chung: Array.from({ length: 12 }, () => question()),
			kd_rieng: Array.from({ length: 4 }, () => Array.from({ length: 10 }, () => question())),
			vcnv: {
				obstacle: '',
				rows: Array.from({ length: 4 }, () => ({ ...question(), keyword: '', char_count: 0 })),
				center: question()
			},
			tt: Array.from({ length: 4 }, () => ({ ...question('image'), time: 20 })),
			vd: Array.from({ length: 4 }, () => ({
				'20': Array.from({ length: 3 }, () => question()),
				'30': Array.from({ length: 3 }, () => question())
			}))
		};
	}

	function normalized(value: QuestionSetData): QuestionSetData {
		const fallback = defaults();
		const normalizeQuestion = (item: Question, types = standardTypes): Question => ({
			...item,
			type: types.includes(item.type) ? item.type : types[0]
		});
		const rows = [...(value.vcnv?.rows ?? [])].slice(0, 4).map((item) => normalizeQuestion(item));
		while (rows.length < 4) rows.push({ ...question(), keyword: '', char_count: 0 });
		const privateRounds = [...(value.kd_rieng ?? [])]
			.slice(0, 4)
			.map((items) => items.map((item) => normalizeQuestion(item)));
		while (privateRounds.length < 4) privateRounds.push([]);
		const finishRounds = [...(value.vd ?? [])].slice(0, 4);
		while (finishRounds.length < 4) finishRounds.push(fallback.vd[finishRounds.length]);
		const normalizedFinishRounds = finishRounds.map((sets) => ({
			'20': (sets['20'] ?? []).map((item) => normalizeQuestion(item, finishTypes)),
			'30': (sets['30'] ?? []).map((item) => normalizeQuestion(item, finishTypes))
		}));
		return {
			kd_chung: (value.kd_chung ?? []).map((item) => normalizeQuestion(item)),
			kd_rieng: privateRounds,
			vcnv: {
				obstacle: value.vcnv?.obstacle ?? '',
				obstacle_asset_id: value.vcnv?.obstacle_asset_id,
				rows,
				center: normalizeQuestion(value.vcnv?.center ?? question())
			},
			tt: (value.tt ?? []).map((item) => normalizeQuestion(item, speedTypes)),
			vd: normalizedFinishRounds
		};
	}

	async function load() {
		if (isNew) return;
		try {
			const loaded = await api.questionSets.get(id);
			name = loaded.questionSet.name;
			category = loaded.questionSet.category;
			version = loaded.questionSet.version;
			data = normalized(loaded.revision.data);
			if (page.url.searchParams.get('duplicate')) {
				name += ' (bản sao)';
				version = 0;
			}
		} catch (cause) {
			error = message(cause);
		} finally {
			loading = false;
		}
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (!data.vcnv.obstacle_asset_id) {
			error = 'VCNV cần một hình ảnh chướng ngại vật.';
			activeSection = 'vcnv';
			return;
		}
		saving = true;
		error = '';
		try {
			const input = { name, category, data, version };
			if (isNew || page.url.searchParams.has('duplicate')) await api.questionSets.create(input);
			else await api.questionSets.update(id, input);
			await goto('#/question-set');
		} catch (cause) {
			error = message(cause);
		} finally {
			saving = false;
		}
	}

	function message(cause: unknown) {
		return cause instanceof Error ? cause.message : 'Không thể lưu đề thi';
	}

	function changed() {
		data = { ...data };
	}

	function remove(list: Question[], index: number) {
		list.splice(index, 1);
		changed();
	}

	function resize(list: Question[], length: number) {
		list.splice(length);
		while (list.length < length) list.push(question());
		changed();
	}

	function setType(value: Question, type: QuestionType) {
		if (value.type !== type) value.asset_id = undefined;
		value.type = type;
		changed();
	}

	function accept(types: QuestionType[]) {
		const values = types.flatMap((type) => {
			if (type === 'image') return ['image/png', 'image/jpeg', 'image/webp'];
			if (type === 'audio') return ['audio/mpeg', 'audio/wav', 'audio/ogg'];
			if (type === 'video') return ['video/mp4'];
			return [];
		});
		return values.join(',');
	}

	function mediaType(file: File): QuestionType | null {
		if (file.type.startsWith('image/')) return 'image';
		if (file.type.startsWith('audio/')) return 'audio';
		if (file.type === 'video/mp4') return 'video';
		return null;
	}

	async function upload(event: Event, allowed: QuestionType[], apply: (asset: Asset) => void) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		const kind = mediaType(file);
		if (!kind || !allowed.includes(kind)) {
			error = `Tệp này phải là ${allowed.map((type) => typeLabels[type].toLowerCase()).join(' hoặc ')}.`;
			input.value = '';
			return;
		}
		uploads += 1;
		error = '';
		try {
			apply(await api.admin.assets.upload(file));
			changed();
		} catch (cause) {
			error = message(cause);
		} finally {
			uploads -= 1;
			input.value = '';
		}
	}
</script>

<svelte:head><title>{isNew ? 'Thêm đề thi' : `Sửa ${name}`} - {brand.appName}</title></svelte:head>

<header class="flex flex-col gap-4">
	<Button variant="ghost" href="#/question-set" class="w-fit"
		><ArrowLeft data-icon="inline-start" />Danh sách đề thi</Button
	>
	<h1 class="text-3xl font-bold tracking-tight">{isNew ? 'Thêm đề thi' : 'Chỉnh sửa đề thi'}</h1>
</header>

{#if error}<Alert.Root variant="destructive"
		><Alert.Title>Không thể lưu</Alert.Title><Alert.Description>{error}</Alert.Description
		></Alert.Root
	>{/if}

{#if loading}
	<div class="flex h-48 items-center justify-center"><Spinner /></div>
{:else}
	<form onsubmit={submit}>
		<Field.FieldGroup>
			<div class="grid gap-5 md:grid-cols-2">
				<Field.Field
					><Field.FieldLabel for="set-name">Tên đề thi</Field.FieldLabel><Input
						id="set-name"
						bind:value={name}
						required
					/></Field.Field
				>
				<Field.Field
					><Field.FieldLabel for="category">Danh mục</Field.FieldLabel><Input
						id="category"
						bind:value={category}
					/></Field.Field
				>
			</div>

			<Tabs.Root bind:value={activeSection}>
				<Tabs.List class="flex h-auto flex-wrap">
					<Tabs.Trigger value="kd_chung">Khởi động chung</Tabs.Trigger>
					<Tabs.Trigger value="kd_rieng">Khởi động riêng</Tabs.Trigger>
					<Tabs.Trigger value="vcnv">VCNV</Tabs.Trigger>
					<Tabs.Trigger value="tt">Tăng tốc</Tabs.Trigger>
					<Tabs.Trigger value="vd">Về đích</Tabs.Trigger>
				</Tabs.List>

				<Tabs.Content value="kd_chung"
					>{@render QuestionList(data.kd_chung, 'Khởi động chung', {})}</Tabs.Content
				>
				<Tabs.Content value="kd_rieng">
					<Tabs.Root bind:value={kdSlot}>
						<Tabs.List
							>{#each [0, 1, 2, 3] as slot (slot)}<Tabs.Trigger value={String(slot)}
									>Thí sinh {slot + 1}</Tabs.Trigger
								>{/each}</Tabs.List
						>
						{#each data.kd_rieng as questions, slot (slot)}
							<Tabs.Content value={String(slot)}
								>{@render QuestionList(questions, `Thí sinh ${slot + 1}`, {
									quickSize: true
								})}</Tabs.Content
							>
						{/each}
					</Tabs.Root>
				</Tabs.Content>

				<Tabs.Content value="vcnv">
					<Field.FieldGroup>
						<div class="grid gap-5 lg:grid-cols-2">
							<Field.Field
								><Field.FieldLabel for="obstacle">Chướng ngại vật</Field.FieldLabel><Input
									id="obstacle"
									bind:value={data.vcnv.obstacle}
									required
								/></Field.Field
							>
							<Field.Field data-invalid={!data.vcnv.obstacle_asset_id}>
								<Field.FieldLabel for="obstacle-image">Hình chướng ngại vật</Field.FieldLabel>
								<Input
									id="obstacle-image"
									type="file"
									accept={accept(['image'])}
									disabled={uploads > 0}
									onchange={(event) =>
										upload(event, ['image'], (asset) => (data.vcnv.obstacle_asset_id = asset.id))}
								/>
								{#if data.vcnv.obstacle_asset_id}{@render AssetPreview(
										data.vcnv.obstacle_asset_id,
										'image'
									)}{/if}
							</Field.Field>
						</div>
						<Field.FieldSet>
							<Field.FieldLegend>Bốn hàng ngang</Field.FieldLegend>
							<Field.FieldDescription
								>Mỗi câu hỏi cố định mở một hàng ngang tương ứng.</Field.FieldDescription
							>
							<Field.FieldGroup
								>{#each data.vcnv.rows as value, index (index)}{@render QuestionEditor(
										value,
										index,
										{ types: standardTypes }
									)}{/each}</Field.FieldGroup
							>
						</Field.FieldSet>
						<Field.FieldSet>
							<Field.FieldLegend>Câu hỏi ô trung tâm</Field.FieldLegend>
							<Field.FieldDescription
								>Mở ô trung tâm của hình chướng ngại vật.</Field.FieldDescription
							>
							{@render QuestionEditor(data.vcnv.center, 4, {
								types: standardTypes,
								label: 'Câu hỏi ô trung tâm'
							})}
						</Field.FieldSet>
					</Field.FieldGroup>
				</Tabs.Content>

				<Tabs.Content value="tt"
					>{@render QuestionList(data.tt, 'Tăng tốc', {
						types: speedTypes,
						showTime: true,
						explanation: true
					})}</Tabs.Content
				>
				<Tabs.Content value="vd">
					<Tabs.Root bind:value={vdSlot}>
						<Tabs.List
							>{#each [0, 1, 2, 3] as slot (slot)}<Tabs.Trigger value={String(slot)}
									>Thí sinh {slot + 1}</Tabs.Trigger
								>{/each}</Tabs.List
						>
						{#each data.vd as sets, slot (slot)}
							<Tabs.Content value={String(slot)}
								><div class="grid gap-6 lg:grid-cols-2">
									{@render QuestionList(sets['20'], 'Gói 20 điểm', {
										types: finishTypes
									})}{@render QuestionList(sets['30'], 'Gói 30 điểm', { types: finishTypes })}
								</div></Tabs.Content
							>
						{/each}
					</Tabs.Root>
				</Tabs.Content>
			</Tabs.Root>

			<div class="flex justify-end">
				<Button type="submit" disabled={saving || uploads > 0 || !name.trim()}
					>{#if saving}<Spinner data-icon="inline-start" />{:else}<Save
							data-icon="inline-start"
						/>{/if}Lưu phiên bản mới</Button
				>
			</div>
		</Field.FieldGroup>
	</form>
{/if}

{#snippet QuestionEditor(value: Question, index: number, options: EditorOptions)}
	<div class="grid gap-4 rounded-lg border p-4">
		<div class="flex items-center justify-between">
			<span class="font-medium">{options.label ?? `Câu ${index + 1}`}</span>
			<Select.Root
				type="single"
				value={value.type}
				onValueChange={(selected) => setType(value, selected as QuestionType)}
			>
				<Select.Trigger class="w-36">{typeLabels[value.type]}</Select.Trigger>
				<Select.Content
					><Select.Group
						>{#each options.types as type (type)}<Select.Item value={type} label={typeLabels[type]}
								>{typeLabels[type]}</Select.Item
							>{/each}</Select.Group
					></Select.Content
				>
			</Select.Root>
		</div>
		<Field.Field>
			<Field.FieldLabel for={`content-${activeSection}-${index}`}>Nội dung</Field.FieldLabel>
			<Textarea id={`content-${activeSection}-${index}`} bind:value={value.content} rows={3} />
			<Field.FieldDescription
				>**đậm**, *nghiêng*, __gạch chân__, ~~gạch bỏ~~, $công thức$ hoặc $$công thức nhiều dòng$$.</Field.FieldDescription
			>
			{#if value.content}<RichText value={value.content} class="rounded-md bg-muted p-3" />{/if}
		</Field.Field>
		<div class="grid gap-4 sm:grid-cols-2">
			<Field.Field
				><Field.FieldLabel for={`answer-${activeSection}-${index}`}
					>Đáp án / từ khóa</Field.FieldLabel
				><Input id={`answer-${activeSection}-${index}`} bind:value={value.answer} /></Field.Field
			>
			{#if options.showTime}<Field.Field
					><Field.FieldLabel for={`time-${activeSection}-${index}`}
						>Thời gian (giây)</Field.FieldLabel
					><Input
						id={`time-${activeSection}-${index}`}
						type="number"
						bind:value={value.time}
						min={1}
						max={300}
					/></Field.Field
				>{/if}
		</div>
		{#if value.type !== 'text'}
			<Field.Field>
				<Field.FieldLabel for={`asset-${activeSection}-${index}`}
					>{typeLabels[value.type]}</Field.FieldLabel
				>
				<Input
					id={`asset-${activeSection}-${index}`}
					type="file"
					accept={accept([value.type])}
					disabled={uploads > 0}
					onchange={(event) => upload(event, [value.type], (asset) => (value.asset_id = asset.id))}
				/>
				{#if value.asset_id}{@render AssetPreview(
						value.asset_id,
						value.type
					)}{:else}<Field.FieldDescription>Tối đa 12 MB.</Field.FieldDescription>{/if}
			</Field.Field>
		{/if}
		{#if options.explanation}
			<Field.Field>
				<Field.FieldLabel for={`explanation-${index}`}
					>Hình giải thích (không bắt buộc)</Field.FieldLabel
				>
				<Input
					id={`explanation-${index}`}
					type="file"
					accept={accept(['image'])}
					disabled={uploads > 0}
					onchange={(event) =>
						upload(event, ['image'], (asset) => (value.explanation_asset_id = asset.id))}
				/>
				{#if value.explanation_asset_id}{@render AssetPreview(
						value.explanation_asset_id,
						'image'
					)}{/if}
			</Field.Field>
		{/if}
	</div>
{/snippet}

{#snippet QuestionList(
	list: Question[],
	title: string,
	options: Omit<EditorOptions, 'types'> & { types?: QuestionType[]; quickSize?: boolean }
)}
	<Field.FieldSet>
		<div class="flex flex-wrap items-center justify-between gap-2">
			<Field.FieldLegend>{title}</Field.FieldLegend>
			<div class="flex items-center gap-2">
				{#if options.quickSize}
					<Select.Root
						type="single"
						value={String(list.length)}
						onValueChange={(value) => resize(list, Number(value))}
					>
						<Select.Trigger class="h-8 w-24">{list.length} câu</Select.Trigger>
						<Select.Content
							><Select.Group
								><Select.Item value="6" label="6 câu">6 câu</Select.Item><Select.Item
									value="10"
									label="10 câu">10 câu</Select.Item
								></Select.Group
							></Select.Content
						>
					</Select.Root>
				{/if}
				<Button
					type="button"
					variant="outline"
					size="sm"
					onclick={() => {
						list.push(question(options.types?.[0] ?? 'text'));
						changed();
					}}><Plus data-icon="inline-start" />Thêm câu</Button
				>
			</div>
		</div>
		<Field.FieldGroup>
			{#each list as value, index (value)}
				<div class="relative">
					{@render QuestionEditor(value, index, {
						types: options.types ?? standardTypes,
						showTime: options.showTime,
						explanation: options.explanation
					})}
					<Popover.Root>
						<Popover.Trigger
							class={`${buttonVariants({ variant: 'ghost', size: 'icon' })} absolute right-4 bottom-4`}
							aria-label="Xóa câu"><Trash2 /></Popover.Trigger
						>
						<Popover.Content align="end" class="w-auto">
							<Popover.Title>Xóa câu hỏi này?</Popover.Title>
							<div class="flex justify-end gap-2">
								<Popover.Close class={buttonVariants({ variant: 'outline', size: 'sm' })}
									>Hủy</Popover.Close
								>
								<Popover.Close
									class={buttonVariants({ variant: 'destructive', size: 'sm' })}
									onclick={() => remove(list, index)}>Xóa</Popover.Close
								>
							</div>
						</Popover.Content>
					</Popover.Root>
				</div>
			{/each}
		</Field.FieldGroup>
	</Field.FieldSet>
{/snippet}

{#snippet AssetPreview(assetID: string, type: QuestionType)}
	<div class="mt-2 overflow-hidden rounded-lg border bg-muted p-2">
		{#if type === 'image'}<img
				class="mx-auto max-h-80 w-full rounded-md object-contain"
				src={`/api/assets/${assetID}`}
				alt="Xem trước tài nguyên"
			/>
		{:else if type === 'video'}<!-- svelte-ignore a11y_media_has_caption --><video
				class="max-h-80 w-full rounded-md"
				src={`/api/assets/${assetID}`}
				controls
			></video>
		{:else if type === 'audio'}<audio class="w-full" src={`/api/assets/${assetID}`} controls
			></audio>{/if}
	</div>
{/snippet}
