<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import * as Field from '$lib/components/ui/field';
	import { Input } from '$lib/components/ui/input';
	import * as Popover from '$lib/components/ui/popover';
	import * as Select from '$lib/components/ui/select';
	import { Spinner } from '$lib/components/ui/spinner';
	import { socket } from '$lib/socket';
	import type { Sound, SoundBus, SoundUpdate } from '$lib/types';
	import MoreHorizontal from '@lucide/svelte/icons/ellipsis';
	import Square from '@lucide/svelte/icons/square';

	let {
		sounds,
		onUpdated
	}: {
		sounds: Sound[];
		onUpdated: (sound: Sound) => void;
	} = $props();

	let drafts = $state<Record<string, SoundUpdate>>({});
	let busySound = $state('');
	let stopping = $state(false);
	let error = $state('');

	$effect(() => {
		for (const sound of sounds) {
			if (!drafts[sound.id]) drafts[sound.id] = toUpdate(sound);
		}
	});

	onMount(() => {
		void socket.sound.getStatus().catch(() => undefined);
		const timer = window.setInterval(
			() => void socket.sound.getStatus().catch(() => undefined),
			5_000
		);
		return () => window.clearInterval(timer);
	});

	async function trigger(sound: Sound): Promise<void> {
		busySound = sound.id;
		error = '';
		try {
			if (sound.bus === 'music' && socket.sound.status.music?.sound_id === sound.id) {
				await socket.sound.stop(socket.sound.status.music.playback_id, sound.fade_out_ms);
			} else {
				await socket.sound.play(sound.id);
			}
			await socket.sound.getStatus();
		} catch (cause) {
			showError(cause);
		} finally {
			busySound = '';
		}
	}

	const busLabels: Record<SoundBus, string> = {
		music: 'Nhạc',
		timer: 'Đếm giờ',
		effects: 'Hiệu ứng'
	};
	const buses: SoundBus[] = ['music', 'timer', 'effects'];

	function busLabel(bus: SoundBus | undefined): string {
		return bus ? (busLabels[bus] ?? bus) : '';
	}

	async function stopBus(bus: SoundBus): Promise<void> {
		stopping = true;
		try {
			await socket.sound.stopBus(bus);
			await socket.sound.getStatus();
		} catch (cause) {
			showError(cause);
		} finally {
			stopping = false;
		}
	}

	async function stopAll(): Promise<void> {
		stopping = true;
		try {
			await socket.sound.stopAll();
			await socket.sound.getStatus();
		} catch (cause) {
			showError(cause);
		} finally {
			stopping = false;
		}
	}

	async function save(sound: Sound): Promise<void> {
		busySound = sound.id;
		try {
			const updated = await api.sounds.update(sound.id, drafts[sound.id]);
			drafts[updated.id] = toUpdate(updated);
			onUpdated(updated);
		} catch (cause) {
			showError(cause);
		} finally {
			busySound = '';
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

	function showError(cause: unknown): void {
		error = cause instanceof Error ? cause.message : 'Không thể điều khiển âm thanh';
	}
</script>

<section class="flex h-full min-h-0 flex-col gap-2 overflow-hidden rounded-md bg-card p-2">
	<header class="flex h-10 shrink-0 items-center justify-between gap-2">
		<div class="flex min-w-0 items-center gap-2">
			<Badge variant="outline">{socket.sound.status.output_count} màn hình có âm thanh</Badge>
			{#if socket.sound.status.music}<span class="max-w-48 truncate text-sm font-medium"
					>Đang phát: {sounds.find((sound) => sound.id === socket.sound.status.music?.sound_id)
						?.name ?? 'Nhạc'}</span
				>{/if}
		</div>
		<div class="flex shrink-0 gap-2">
			{#each buses as bus (bus)}
				<Button size="sm" variant="outline" disabled={stopping} onclick={() => stopBus(bus)}
					><Square data-icon="inline-start" />Dừng {busLabel(bus).toLowerCase()}</Button
				>
			{/each}
			<Button size="sm" variant="destructive" disabled={stopping} onclick={stopAll}
				><Square data-icon="inline-start" />Dừng tất cả</Button
			>
		</div>
	</header>

	{#if error}<Alert.Root variant="destructive"
			><Alert.Title>Âm thanh lỗi</Alert.Title><Alert.Description>{error}</Alert.Description
			></Alert.Root
		>{/if}

	<div
		class="grid min-h-0 flex-1 auto-rows-[5.5rem] grid-cols-2 gap-2 overflow-y-auto pr-1 lg:grid-cols-3 xl:grid-cols-4"
	>
		{#each sounds as sound (sound.id)}
			<div class="relative min-w-0">
				<Button
					variant={socket.sound.status.music?.sound_id === sound.id ? 'default' : 'secondary'}
					class="h-full w-full min-w-0 flex-col items-start justify-center gap-1 overflow-hidden px-4 pr-11 text-left whitespace-nowrap"
					disabled={busySound === sound.id}
					onclick={() => trigger(sound)}
				>
					{#if busySound === sound.id}<Spinner />{:else}<span
							class="w-full truncate font-semibold"
							title={sound.name}>{sound.name}</span
						><span class="w-full truncate text-xs opacity-70"
							>{busLabel(sound.bus)} · {sound.category}</span
						>{/if}
				</Button>
				<Popover.Root>
					<Popover.Trigger
						class={`${buttonVariants({ variant: 'ghost', size: 'icon-sm' })} absolute top-1 right-1`}
						aria-label={`Cài đặt ${sound.name}`}><MoreHorizontal /></Popover.Trigger
					>
					<Popover.Content align="end" class="w-80">
						<Popover.Header><Popover.Title>Mặc định khi phát</Popover.Title></Popover.Header>
						<Field.FieldGroup>
							<Field.Field
								><Field.FieldLabel>Kênh</Field.FieldLabel><Select.Root
									type="single"
									value={drafts[sound.id]?.bus}
									onValueChange={(value) => (drafts[sound.id].bus = value as SoundBus)}
									><Select.Trigger class="w-full">{busLabel(drafts[sound.id]?.bus)}</Select.Trigger
									><Select.Content
										><Select.Group
											>{#each buses as bus (bus)}<Select.Item value={bus} label={busLabel(bus)}
													>{busLabel(bus)}</Select.Item
												>{/each}</Select.Group
										></Select.Content
									></Select.Root
								></Field.Field
							>
							<div class="grid grid-cols-3 gap-2">
								<Field.Field
									><Field.FieldLabel for={`pad-volume-${sound.id}`}>Âm lượng</Field.FieldLabel
									><Input
										id={`pad-volume-${sound.id}`}
										type="number"
										min="0"
										max="1"
										step="0.05"
										bind:value={drafts[sound.id].volume}
									/></Field.Field
								>
								<Field.Field
									><Field.FieldLabel for={`pad-in-${sound.id}`}>Fade in</Field.FieldLabel><Input
										id={`pad-in-${sound.id}`}
										type="number"
										min="0"
										max="10000"
										bind:value={drafts[sound.id].fade_in_ms}
									/></Field.Field
								>
								<Field.Field
									><Field.FieldLabel for={`pad-out-${sound.id}`}>Fade out</Field.FieldLabel><Input
										id={`pad-out-${sound.id}`}
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
								>{drafts[sound.id]?.loop ? 'Lặp: Bật' : 'Lặp: Tắt'}</Button
							>
							<div class="flex justify-end">
								<Popover.Close class={buttonVariants({ size: 'sm' })} onclick={() => save(sound)}
									>Lưu</Popover.Close
								>
							</div>
						</Field.FieldGroup>
					</Popover.Content>
				</Popover.Root>
			</div>
		{/each}
	</div>
</section>
