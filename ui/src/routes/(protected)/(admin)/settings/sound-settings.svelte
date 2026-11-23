<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Field from '$lib/components/ui/field';
	import { Input } from '$lib/components/ui/input';
	import * as Popover from '$lib/components/ui/popover';
	import * as Select from '$lib/components/ui/select';
	import { Spinner } from '$lib/components/ui/spinner';
	import * as Table from '$lib/components/ui/table';
	import type { SiteSettings, Sound, SoundBus, SoundUpdate } from '$lib/types';
	import { DEFAULT_SCORING } from '$lib/operator/scoring';
	import MoreHorizontal from '@lucide/svelte/icons/ellipsis';
	import Upload from '@lucide/svelte/icons/upload';

	const emptySettings: SiteSettings = {
		event_name: '',
		host_name: '',
		sound: {
			enabled: true,
			master_volume: 0.8,
			music_volume: 0.8,
			effects_volume: 1,
			transition_fade_ms: 120,
			bell_sound_id: 'builtin.kd.bell'
		},
		scoring: structuredClone(DEFAULT_SCORING)
	};

	let settings = $state<SiteSettings>(emptySettings);
	let sounds = $state<Sound[]>([]);
	let drafts = $state<Record<string, SoundUpdate>>({});
	let uploadFile = $state<File | null>(null);
	let uploadName = $state('');
	let uploadCategory = $state('main');
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let notice = $state('');

	onMount(() => void load());

	async function load(): Promise<void> {
		loading = true;
		try {
			[settings, sounds] = await Promise.all([api.settings.get(), api.sounds.manage()]);
			drafts = Object.fromEntries(sounds.map((sound) => [sound.id, toUpdate(sound)]));
		} catch (cause) {
			showError(cause);
		} finally {
			loading = false;
		}
	}

	async function saveMixer(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		await run(async () => {
			settings = await api.settings.update(settings);
			notice = 'Đã lưu cài đặt âm thanh.';
		});
	}

	async function upload(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (!uploadFile) return;
		await run(async () => {
			await api.sounds.upload(uploadFile as File, {
				name: uploadName.trim() || uploadFile?.name.replace(/\.[^.]+$/, '') || 'Âm thanh',
				category: uploadCategory.trim() || 'main',
				sort_order: sounds.length,
				bus: 'effects',
				volume: 1,
				loop: false,
				fade_in_ms: 0,
				fade_out_ms: 120,
				enabled: true
			});
			uploadFile = null;
			uploadName = '';
			await load();
			notice = 'Đã tải âm thanh lên.';
		});
	}

	async function saveSound(sound: Sound): Promise<void> {
		await run(async () => {
			const updated = await api.sounds.update(sound.id, drafts[sound.id]);
			sounds = sounds.map((item) => (item.id === updated.id ? updated : item));
			drafts[updated.id] = toUpdate(updated);
			notice = `Đã lưu ${updated.name}.`;
		});
	}

	async function toggleSound(sound: Sound): Promise<void> {
		drafts[sound.id].enabled = !sound.enabled;
		await saveSound(sound);
	}

	async function archiveSound(sound: Sound): Promise<void> {
		await run(async () => {
			await api.sounds.archive(sound.id);
			sounds = sounds.filter((item) => item.id !== sound.id);
			notice = `Đã xóa ${sound.name}.`;
		});
	}

	async function run(action: () => Promise<void>): Promise<void> {
		saving = true;
		error = '';
		notice = '';
		try {
			await action();
		} catch (cause) {
			showError(cause);
		} finally {
			saving = false;
		}
	}

	function toUpdate(sound: Sound): SoundUpdate {
		return {
			name: sound.name,
			category: sound.category,
			sort_order: sound.sort_order,
			bus: sound.bus,
			volume: sound.volume,
			loop: sound.loop,
			fade_in_ms: sound.fade_in_ms,
			fade_out_ms: sound.fade_out_ms,
			enabled: sound.enabled
		};
	}

	function busLabel(bus: SoundBus): string {
		return bus === 'music' ? 'Nhạc' : bus === 'timer' ? 'Đếm giờ' : 'Hiệu ứng';
	}

	function showError(cause: unknown): void {
		error = cause instanceof Error ? cause.message : 'Không thể hoàn thành thao tác';
		notice = '';
	}
</script>

{#if error}
	<Alert.Root variant="destructive"
		><Alert.Title>Không thể hoàn thành</Alert.Title><Alert.Description>{error}</Alert.Description
		></Alert.Root
	>
{:else if notice}
	<Alert.Root
		><Alert.Title>Thành công</Alert.Title><Alert.Description>{notice}</Alert.Description
		></Alert.Root
	>
{/if}

{#if loading}
	<Spinner />
{:else}
	<Card.Root>
		<Card.Header><Card.Title>Bộ trộn âm thanh</Card.Title></Card.Header>
		<Card.Content>
			<form onsubmit={saveMixer}>
				<Field.FieldGroup>
					<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
						<Field.Field>
							<Field.FieldLabel for="master-volume">Âm lượng tổng</Field.FieldLabel>
							<Input
								id="master-volume"
								type="number"
								min="0"
								max="1"
								step="0.05"
								bind:value={settings.sound.master_volume}
							/>
						</Field.Field>
						<Field.Field>
							<Field.FieldLabel for="music-volume">Nhạc</Field.FieldLabel>
							<Input
								id="music-volume"
								type="number"
								min="0"
								max="1"
								step="0.05"
								bind:value={settings.sound.music_volume}
							/>
						</Field.Field>
						<Field.Field>
							<Field.FieldLabel for="effects-volume">Hiệu ứng</Field.FieldLabel>
							<Input
								id="effects-volume"
								type="number"
								min="0"
								max="1"
								step="0.05"
								bind:value={settings.sound.effects_volume}
							/>
						</Field.Field>
						<Field.Field>
							<Field.FieldLabel for="transition-fade">Fade khi đổi màn hình (ms)</Field.FieldLabel>
							<Input
								id="transition-fade"
								type="number"
								min="0"
								max="10000"
								step="10"
								bind:value={settings.sound.transition_fade_ms}
							/>
						</Field.Field>
					</div>
					<Field.Field>
						<Field.FieldLabel for="bell-sound">Tiếng chuông (Khởi động)</Field.FieldLabel>
						<Select.Root type="single" bind:value={settings.sound.bell_sound_id}>
							<Select.Trigger id="bell-sound" class="w-full max-w-md"
								>{sounds.find((sound) => sound.id === settings.sound.bell_sound_id)?.name ??
									'Chọn tiếng chuông'}</Select.Trigger
							>
							<Select.Content
								><Select.Group
									>{#each sounds.filter((sound) => sound.enabled) as sound (sound.id)}<Select.Item
											value={sound.id}
											label={sound.name}>{sound.name}</Select.Item
										>{/each}</Select.Group
								></Select.Content
							>
						</Select.Root>
					</Field.Field>
					<div class="flex flex-wrap gap-2">
						<Button
							type="button"
							variant={settings.sound.enabled ? 'secondary' : 'destructive'}
							onclick={() => (settings.sound.enabled = !settings.sound.enabled)}
							>{settings.sound.enabled ? 'Âm thanh đang bật' : 'Âm thanh đang tắt'}</Button
						>
						<Button type="submit" disabled={saving}
							>{#if saving}<Spinner data-icon="inline-start" />{/if}Lưu cài đặt</Button
						>
					</div>
				</Field.FieldGroup>
			</form>
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header><Card.Title>Thư viện âm thanh</Card.Title></Card.Header>
		<Card.Content class="flex flex-col gap-4">
			<form onsubmit={upload}>
				<Field.FieldGroup class="grid md:grid-cols-[1fr_1fr_1fr_auto] md:items-end">
					<Field.Field
						><Field.FieldLabel for="sound-file">Tệp âm thanh</Field.FieldLabel><Input
							id="sound-file"
							type="file"
							accept="audio/mpeg,audio/wav,audio/ogg,audio/mp4,.m4a"
							onchange={(event) => (uploadFile = event.currentTarget.files?.[0] ?? null)}
						/></Field.Field
					>
					<Field.Field
						><Field.FieldLabel for="sound-name">Tên</Field.FieldLabel><Input
							id="sound-name"
							bind:value={uploadName}
						/></Field.Field
					>
					<Field.Field
						><Field.FieldLabel for="sound-category">Nhóm</Field.FieldLabel><Input
							id="sound-category"
							bind:value={uploadCategory}
							required
						/></Field.Field
					>
					<Button type="submit" disabled={saving || !uploadFile}
						><Upload data-icon="inline-start" />Tải lên</Button
					>
				</Field.FieldGroup>
			</form>

			<div class="overflow-hidden rounded-md border">
				<Table.Root>
					<Table.Header
						><Table.Row
							><Table.Head>Tên</Table.Head><Table.Head>Nhóm</Table.Head><Table.Head>Kênh</Table.Head
							><Table.Head>Trạng thái</Table.Head><Table.Head></Table.Head></Table.Row
						></Table.Header
					>
					<Table.Body>
						{#each sounds as sound (sound.id)}
							<Table.Row>
								<Table.Cell class="font-medium">{sound.name}</Table.Cell>
								<Table.Cell>{sound.category}</Table.Cell>
								<Table.Cell><Badge variant="outline">{busLabel(sound.bus)}</Badge></Table.Cell>
								<Table.Cell
									><Badge variant={sound.enabled ? 'secondary' : 'outline'}
										>{sound.enabled ? 'Đang bật' : 'Đã tắt'}</Badge
									></Table.Cell
								>
								<Table.Cell class="text-right">
									<div class="flex justify-end gap-2">
										<Button
											size="sm"
											variant="outline"
											disabled={saving}
											onclick={() => toggleSound(sound)}>{sound.enabled ? 'Tắt' : 'Bật'}</Button
										>
										<Popover.Root>
											<Popover.Trigger
												class={buttonVariants({ variant: 'ghost', size: 'icon-sm' })}
												aria-label={`Sửa ${sound.name}`}><MoreHorizontal /></Popover.Trigger
											>
											<Popover.Content align="end" class="w-80">
												<Popover.Header
													><Popover.Title>Cài đặt âm thanh</Popover.Title></Popover.Header
												>
												<Field.FieldGroup>
													<Field.Field
														><Field.FieldLabel for={`name-${sound.id}`}>Tên</Field.FieldLabel><Input
															id={`name-${sound.id}`}
															bind:value={drafts[sound.id].name}
														/></Field.Field
													>
													<Field.Field
														><Field.FieldLabel for={`category-${sound.id}`}>Nhóm</Field.FieldLabel
														><Input
															id={`category-${sound.id}`}
															bind:value={drafts[sound.id].category}
														/></Field.Field
													>
													<Field.Field
														><Field.FieldLabel>Kênh</Field.FieldLabel><Select.Root
															type="single"
															value={drafts[sound.id].bus}
															onValueChange={(value) => (drafts[sound.id].bus = value as SoundBus)}
															><Select.Trigger class="w-full"
																>{busLabel(drafts[sound.id].bus)}</Select.Trigger
															><Select.Content
																><Select.Group
																	><Select.Item value="music" label="Nhạc">Nhạc</Select.Item
																	><Select.Item value="timer" label="Đếm giờ">Đếm giờ</Select.Item
																	><Select.Item value="effects" label="Hiệu ứng"
																		>Hiệu ứng</Select.Item
																	></Select.Group
																></Select.Content
															></Select.Root
														></Field.Field
													>
													<div class="grid grid-cols-3 gap-2">
														<Field.Field
															><Field.FieldLabel for={`volume-${sound.id}`}
																>Âm lượng</Field.FieldLabel
															><Input
																id={`volume-${sound.id}`}
																type="number"
																min="0"
																max="1"
																step="0.05"
																bind:value={drafts[sound.id].volume}
															/></Field.Field
														>
														<Field.Field
															><Field.FieldLabel for={`fade-in-${sound.id}`}
																>Fade in</Field.FieldLabel
															><Input
																id={`fade-in-${sound.id}`}
																type="number"
																min="0"
																max="10000"
																bind:value={drafts[sound.id].fade_in_ms}
															/></Field.Field
														>
														<Field.Field
															><Field.FieldLabel for={`fade-out-${sound.id}`}
																>Fade out</Field.FieldLabel
															><Input
																id={`fade-out-${sound.id}`}
																type="number"
																min="0"
																max="10000"
																bind:value={drafts[sound.id].fade_out_ms}
															/></Field.Field
														>
													</div>
													<Button
														type="button"
														variant="outline"
														onclick={() => (drafts[sound.id].loop = !drafts[sound.id].loop)}
														>{drafts[sound.id].loop ? 'Lặp: Bật' : 'Lặp: Tắt'}</Button
													>
													<div class="flex justify-between gap-2">
														{#if !sound.builtin}<Popover.Root
																><Popover.Trigger
																	class={buttonVariants({ variant: 'destructive', size: 'sm' })}
																	>Xóa</Popover.Trigger
																><Popover.Content
																	><Popover.Header
																		><Popover.Title>Xóa âm thanh?</Popover.Title></Popover.Header
																	>
																	<div class="flex justify-end gap-2">
																		<Popover.Close
																			class={buttonVariants({ variant: 'outline', size: 'sm' })}
																			>Hủy</Popover.Close
																		><Popover.Close
																			class={buttonVariants({ variant: 'destructive', size: 'sm' })}
																			onclick={() => archiveSound(sound)}>Xóa</Popover.Close
																		>
																	</div></Popover.Content
																></Popover.Root
															>{:else}<span></span>{/if}
														<Popover.Close
															class={buttonVariants({ size: 'sm' })}
															onclick={() => saveSound(sound)}>Lưu</Popover.Close
														>
													</div>
												</Field.FieldGroup>
											</Popover.Content>
										</Popover.Root>
									</div>
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			</div>
		</Card.Content>
	</Card.Root>
{/if}
