<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { DEFAULT_SCORING } from '$lib/operator/scoring';
	import type { ScoringRules } from '$lib/types';

	type Props = {
		value: ScoringRules;
		saving?: boolean;
		/** Return false when the save failed so unsaved edits are kept. */
		onsave: (rules: ScoringRules) => boolean | void | Promise<boolean | void>;
		dense?: boolean;
	};
	let { value, saving = false, onsave, dense = false }: Props = $props();

	type Kind = 'points' | 'tier' | 'percent' | 'nudge';
	type Item = {
		label: string;
		kind: Kind;
		get: (r: ScoringRules) => number;
		set: (r: ScoringRules, v: number) => void;
	};
	type Section = { title: string; items: Item[]; floorZero?: boolean };

	const RANGES: Record<Kind, [number, number]> = {
		points: [-1000, 1000],
		tier: [1, 1000],
		percent: [-1000, 1000],
		nudge: [1, 1000]
	};

	const item = (
		label: string,
		kind: Kind,
		get: (r: ScoringRules) => number,
		set: (r: ScoringRules, v: number) => void
	): Item => ({ label, kind, get, set });

	const sections: Section[] = [
		{
			title: 'Khởi động',
			floorZero: true,
			items: [
				item('Riêng — đúng', 'points', (r) => r.khoi_dong.individual_correct, (r, v) => (r.khoi_dong.individual_correct = v)),
				item('Riêng — sai', 'points', (r) => r.khoi_dong.individual_wrong, (r, v) => (r.khoi_dong.individual_wrong = v)),
				item('Chung — đúng', 'points', (r) => r.khoi_dong.common_correct, (r, v) => (r.khoi_dong.common_correct = v)),
				item('Chung — sai', 'points', (r) => r.khoi_dong.common_wrong, (r, v) => (r.khoi_dong.common_wrong = v))
			]
		},
		{
			title: 'Vượt chướng ngại vật',
			items: [
				item('Hàng ngang — đúng', 'points', (r) => r.vcnv.row_correct, (r, v) => (r.vcnv.row_correct = v)),
				item('Hàng ngang — sai', 'points', (r) => r.vcnv.row_wrong, (r, v) => (r.vcnv.row_wrong = v)),
				item('Ô trung tâm — đúng', 'points', (r) => r.vcnv.center_correct, (r, v) => (r.vcnv.center_correct = v)),
				item('Ô trung tâm — sai', 'points', (r) => r.vcnv.center_wrong, (r, v) => (r.vcnv.center_wrong = v)),
				...[0, 1, 2, 3].map((i) =>
					item(`CNV đúng trong hàng ${i + 1}`, 'points', (r) => r.vcnv.obstacle_by_rows[i], (r, v) => (r.vcnv.obstacle_by_rows[i] = v))
				),
				item('CNV sau gợi ý trung tâm', 'points', (r) => r.vcnv.obstacle_after_center, (r, v) => (r.vcnv.obstacle_after_center = v)),
				item('CNV sai', 'points', (r) => r.vcnv.obstacle_wrong, (r, v) => (r.vcnv.obstacle_wrong = v))
			]
		},
		{
			title: 'Tăng tốc',
			items: [
				...[0, 1, 2, 3].map((i) =>
					item(`Hạng ${i + 1}`, 'points', (r) => r.tang_toc.ranks[i], (r, v) => (r.tang_toc.ranks[i] = v))
				),
				item('Sai', 'points', (r) => r.tang_toc.wrong, (r, v) => (r.tang_toc.wrong = v))
			]
		},
		{
			title: 'Về đích',
			items: [
				item('Gói 1', 'tier', (r) => r.ve_dich.tiers[0], (r, v) => (r.ve_dich.tiers[0] = v)),
				item('Gói 2', 'tier', (r) => r.ve_dich.tiers[1], (r, v) => (r.ve_dich.tiers[1] = v)),
				item('Đúng', 'percent', (r) => r.ve_dich.correct_percent, (r, v) => (r.ve_dich.correct_percent = v)),
				item('Sai', 'percent', (r) => r.ve_dich.wrong_percent, (r, v) => (r.ve_dich.wrong_percent = v)),
				item('Ngôi sao đúng', 'percent', (r) => r.ve_dich.star_correct_percent, (r, v) => (r.ve_dich.star_correct_percent = v)),
				item('Ngôi sao sai', 'percent', (r) => r.ve_dich.star_wrong_percent, (r, v) => (r.ve_dich.star_wrong_percent = v)),
				item('Cướp đúng', 'percent', (r) => r.ve_dich.steal_correct_percent, (r, v) => (r.ve_dich.steal_correct_percent = v)),
				item('Cướp sai', 'percent', (r) => r.ve_dich.steal_wrong_percent, (r, v) => (r.ve_dich.steal_wrong_percent = v)),
				item('Thí sinh chính bị cướp', 'percent', (r) => r.ve_dich.stolen_percent, (r, v) => (r.ve_dich.stolen_percent = v)),
				item('Bị cướp khi có ngôi sao', 'percent', (r) => r.ve_dich.stolen_star_percent, (r, v) => (r.ve_dich.stolen_star_percent = v))
			]
		},
		{
			title: 'Bảng điều khiển',
			items: [
				item('Nút trừ', 'nudge', (r) => r.adjust.down, (r, v) => (r.adjust.down = v)),
				item('Nút cộng', 'nudge', (r) => r.adjust.up, (r, v) => (r.adjust.up = v))
			]
		}
	];

	let draft = $state<ScoringRules>(structuredClone(DEFAULT_SCORING));
	let dirty = $state(false);

	$effect.pre(() => {
		const next = $state.snapshot(value) as ScoringRules;
		if (!dirty) draft = structuredClone(next);
	});

	function errorFor(entry: Item, rules: ScoringRules): string {
		const v = entry.get(rules);
		const [min, max] = RANGES[entry.kind];
		if (typeof v !== 'number' || !Number.isInteger(v)) return 'Nhập số nguyên';
		if (v < min || v > max) return `Từ ${min} đến ${max}`;
		if (entry.kind === 'tier' && rules.ve_dich.tiers[0] === rules.ve_dich.tiers[1])
			return 'Hai gói phải khác nhau';
		return '';
	}

	let errors = $derived(
		new Map(sections.flatMap((s) => s.items.map((i) => [i, errorFor(i, draft)] as const)))
	);
	let invalid = $derived([...errors.values()].some(Boolean));

	function update(entry: Item, raw: string): void {
		entry.set(draft, raw.trim() === '' ? Number.NaN : Number(raw));
		dirty = true;
	}

	function reset(): void {
		draft = structuredClone(DEFAULT_SCORING);
		dirty = true;
	}

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (invalid || saving) return;
		const saved = await onsave($state.snapshot(draft) as ScoringRules);
		if (saved !== false) dirty = false;
	}
</script>

<form class="grid gap-4" onsubmit={submit}>
	<div class={dense ? 'grid gap-3 md:grid-cols-2' : 'grid gap-4 lg:grid-cols-2'}>
		{#each sections as section (section.title)}
			<fieldset class="border-border bg-card rounded-lg border {dense ? 'p-3' : 'p-4'}">
				<legend class="text-foreground px-1 text-sm font-semibold">{section.title}</legend>
				<div class="grid {dense ? 'gap-1.5' : 'gap-2'}">
					{#each section.items as entry (entry.label)}
						{@const message = errors.get(entry)}
						<div class="grid gap-0.5">
							<label class="flex items-center justify-between gap-3 text-sm">
								<span class="text-muted-foreground">{entry.label}</span>
								<span class="flex items-center gap-1">
									<Input
										type="number"
										step="1"
										inputmode="numeric"
										class="h-8 w-20 text-right tabular-nums"
										min={RANGES[entry.kind][0]}
										max={RANGES[entry.kind][1]}
										aria-invalid={message ? 'true' : undefined}
										value={Number.isNaN(entry.get(draft)) ? '' : entry.get(draft)}
										oninput={(e) => update(entry, e.currentTarget.value)}
									/>
									{#if entry.kind === 'percent'}
										<span class="text-muted-foreground w-3 text-sm">%</span>
									{:else if section.title === 'Về đích'}
										<span class="w-3"></span>
									{/if}
								</span>
							</label>
							{#if message}
								<p class="text-destructive text-right text-xs">{message}</p>
							{/if}
						</div>
					{/each}
					{#if section.floorZero}
						<label class="text-muted-foreground flex items-center gap-2 pt-1 text-sm">
							<input
								type="checkbox"
								class="accent-primary size-4"
								checked={draft.khoi_dong.common_floor_zero}
								onchange={(e) => {
									draft.khoi_dong.common_floor_zero = e.currentTarget.checked;
									dirty = true;
								}}
							/>
							Không trừ xuống dưới 0
						</label>
					{/if}
				</div>
			</fieldset>
		{/each}
	</div>
	<div class="flex flex-wrap justify-end gap-2">
		<Button type="button" variant="outline" onclick={reset} disabled={saving}>Khôi phục mặc định</Button>
		<Button type="submit" disabled={invalid || saving}>Lưu luật điểm</Button>
	</div>
</form>
