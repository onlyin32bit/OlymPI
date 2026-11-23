<script lang="ts">
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import * as Popover from '$lib/components/ui/popover';
	import { Spinner } from '$lib/components/ui/spinner';
	import { socket } from '$lib/socket';
	import { cn } from '$lib/utils';
	import Backspace from '@lucide/svelte/icons/delete';
	import BellRing from '@lucide/svelte/icons/bell-ring';
	import Lock from '@lucide/svelte/icons/lock';
	import LockOpen from '@lucide/svelte/icons/lock-open';
	import Minus from '@lucide/svelte/icons/minus';
	import Plus from '@lucide/svelte/icons/plus';

	let busySlot = $state(0);
	let submitting = $state(false);
	let error = $state('');
	let drafts = $state<Record<number, number>>({});
	let negative = $state<Record<number, boolean>>({});
	let activeMatchID = '';
	let runtime = $derived(socket.match.current);
	let hasDraft = $derived(Object.values(drafts).some((value) => value !== 0));

	$effect(() => {
		const matchID = runtime?.match_id ?? '';
		if (matchID === activeMatchID) return;
		activeMatchID = matchID;
		drafts = {};
		negative = {};
	});

	function changeDraft(slot: number, delta: number): void {
		const next = clamp((drafts[slot] ?? 0) + delta);
		drafts[slot] = next;
		negative[slot] = next < 0;
	}

	function appendDigit(slot: number, digit: number): void {
		const absolute = Math.min(1000, Math.abs(drafts[slot] ?? 0) * 10 + digit);
		drafts[slot] = (negative[slot] ? -1 : 1) * absolute;
	}

	function toggleSign(slot: number): void {
		negative[slot] = !negative[slot];
		drafts[slot] = (negative[slot] ? -1 : 1) * Math.abs(drafts[slot] ?? 0);
	}

	function backspace(slot: number): void {
		const absolute = Math.floor(Math.abs(drafts[slot] ?? 0) / 10);
		drafts[slot] = (negative[slot] ? -1 : 1) * absolute;
	}

	function formatDraft(value: number): string {
		if (value > 0) return `+${value}`;
		if (value < 0) return String(value);
		return '±0';
	}

	function clamp(value: number): number {
		return Math.max(-1000, Math.min(1000, value));
	}

	async function submitScores(): Promise<void> {
		const adjustments = (runtime?.contestants ?? [])
			.map((contestant) => ({ slot: contestant.slot, delta: drafts[contestant.slot] ?? 0 }))
			.filter((adjustment) => adjustment.delta !== 0);
		if (!adjustments.length) return;
		submitting = true;
		error = '';
		try {
			await socket.match.adjustScores(adjustments, 'Cập nhật điểm tại bảng điều khiển');
			drafts = {};
			negative = {};
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Không thể cập nhật điểm';
		} finally {
			submitting = false;
		}
	}

	async function setLock(slot: number, locked: boolean): Promise<void> {
		busySlot = slot;
		error = '';
		try {
			await socket.match.setContestantLock(slot, locked);
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Không thể đổi trạng thái thí sinh';
		} finally {
			busySlot = 0;
		}
	}
</script>

<div class="flex h-full min-h-0 flex-col overflow-hidden rounded-md border">
	{#if error}<p class="border-b bg-destructive/10 px-3 py-2 text-sm text-destructive">
			{error}
		</p>{/if}
	<div class="min-h-0 flex-1 overflow-auto">
		<table class="h-full w-full text-left text-gray-800">
			<thead class="sticky top-0 bg-white">
				<tr class="border-b-2 text-xs font-bold text-gray-400">
					<th class="px-3 py-2">THÍ SINH</th><th class="px-3 py-2">CÂU TRẢ LỜI</th>
					<th class="px-3 py-2">THỜI GIAN</th><th class="w-60 min-w-60 px-3 py-2 text-center"
						>ĐIỂM</th
					><th class="px-3 py-2 text-right">KHÓA</th>
				</tr>
			</thead>
			<tbody>
				{#each runtime?.contestants ?? [] as contestant (contestant.slot)}
					<tr class="border-t">
						<td class="px-3 py-2"
							><div class="flex items-center gap-2">
								{#if contestant.has_rung}<BellRing
										class="size-4 text-amber-500"
										fill="currentColor"
									/>{/if}
								<div>
									<p class="font-semibold">{contestant.name}</p>
									<p class="text-xs text-muted-foreground">
										{contestant.class || `Vị trí ${contestant.slot}`}
									</p>
								</div>
							</div></td
						>
						<td class="max-w-48 truncate px-3 py-2">{contestant.answer || 'Chưa trả lời'}</td>
						<td class="px-3 py-2 font-mono tabular-nums"
							>{contestant.time_ms ? `${(contestant.time_ms / 1000).toFixed(3)}s` : '—'}</td
						>
						<td class="w-60 min-w-60 px-3 py-2"
							><div class="flex w-full items-center justify-center gap-1">
								<Button
									size="icon-sm"
									variant="outline"
									aria-label={`Tạm trừ 5 điểm của ${contestant.name}`}
									disabled={submitting}
									onclick={() => changeDraft(contestant.slot, -5)}><Minus /></Button
								><strong class="w-14 shrink-0 text-center font-mono text-xl tabular-nums"
									>{contestant.score}</strong
								><Popover.Root>
									<Popover.Trigger
										class={cn(
											buttonVariants({ variant: 'ghost' }),
											'-my-0.5 h-auto w-16 shrink-0 bg-muted/40 px-1.5 py-0.5 font-mono text-xl leading-none tabular-nums hover:bg-muted/60'
										)}
										aria-label={`Nhập điểm tạm cho ${contestant.name}`}
										disabled={submitting}
										>{formatDraft(drafts[contestant.slot] ?? 0)}</Popover.Trigger
									>
									<Popover.Content class="w-52" align="center">
										<Popover.Header><Popover.Title>Điểm tạm</Popover.Title></Popover.Header>
										<div class="grid grid-cols-3 gap-2">
											{#each [1, 2, 3, 4, 5, 6, 7, 8, 9] as digit (digit)}
												<Button
													variant="outline"
													class="h-10"
													onclick={() => appendDigit(contestant.slot, digit)}>{digit}</Button
												>
											{/each}
											<Button
												variant={negative[contestant.slot] ? 'default' : 'outline'}
												class="h-10"
												onclick={() => toggleSign(contestant.slot)}>+/−</Button
											><Button
												variant="outline"
												class="h-10"
												onclick={() => appendDigit(contestant.slot, 0)}>0</Button
											><Button
												variant="outline"
												class="h-10"
												aria-label="Xóa một chữ số"
												onclick={() => backspace(contestant.slot)}><Backspace /></Button
											>
										</div>
									</Popover.Content>
								</Popover.Root>
								<Button
									size="icon-sm"
									variant="outline"
									aria-label={`Tạm cộng 10 điểm cho ${contestant.name}`}
									disabled={submitting}
									onclick={() => changeDraft(contestant.slot, 10)}><Plus /></Button
								>
							</div></td
						>
						<td class="px-3 py-2 text-right"
							><Button
								size="icon-sm"
								variant={contestant.locked ? 'destructive' : 'outline'}
								aria-label={contestant.locked
									? `Mở khóa ${contestant.name}`
									: `Khóa ${contestant.name}`}
								disabled={submitting || busySlot === contestant.slot}
								onclick={() => setLock(contestant.slot, !contestant.locked)}
								>{#if contestant.locked}<Lock />{:else}<LockOpen />{/if}</Button
							></td
						>
					</tr>
				{/each}
				{#if !runtime?.match_id}<tr
						><td colspan="5" class="h-32 text-center text-muted-foreground">Chưa chọn trận đấu.</td
						></tr
					>{/if}
			</tbody>
		</table>
	</div>
	<div class="flex shrink-0 justify-end border-t p-2">
		<Button disabled={!hasDraft || submitting || !runtime?.match_id} onclick={submitScores}>
			{#if submitting}<Spinner data-icon="inline-start" />{/if}Cập nhật điểm
		</Button>
	</div>
</div>
