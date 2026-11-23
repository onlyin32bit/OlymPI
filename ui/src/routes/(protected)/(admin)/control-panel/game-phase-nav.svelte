<script lang="ts">
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Spinner } from '$lib/components/ui/spinner';
	import type { Phases } from '$lib/types';
	import type { GamePhase } from '.';

	let {
		phases,
		active,
		applying,
		error,
		onSelect
	}: {
		phases: readonly GamePhase[];
		active: Phases;
		applying: Phases | null;
		error: string;
		onSelect: (phase: Phases) => void;
	} = $props();
	const shortLabels: Record<Phases, string> = {
		idle: 'Tĩnh',
		logo: 'Logo',
		main: 'Chính',
		score: 'Điểm',
		khoi_dong: 'Khởi động',
		vcnv: 'VCNV',
		tang_toc: 'Tăng tốc',
		ve_dich: 'Về đích'
	};
</script>

<nav
	class="flex flex-none flex-col border-b border-slate-200 bg-slate-50 p-0.5"
	aria-label="Vòng thi"
>
	<div class="grid min-w-0 grid-cols-7 gap-0.5">
		{#each phases as phase (phase.id)}
			{@const isActive = phase.id === active}
			<Button
				variant="ghost"
				class={`h-8 min-w-0 gap-1 rounded-sm px-1 py-0 text-[13px] leading-tight ${isActive ? 'bg-blue-100 text-blue-800 hover:bg-blue-100' : 'text-slate-600'}`}
				aria-current={isActive ? 'step' : undefined}
				aria-label={isActive ? `${phase.label} — Đang phát` : phase.label}
				title={phase.label}
				disabled={applying !== null}
				onclick={() => onSelect(phase.id)}
			>
				<span>{shortLabels[phase.id]}</span>
				{#if applying === phase.id}
					<Spinner />
				{:else if phase.id === active}
					<span class="size-1.5 shrink-0 rounded-full bg-red-600" aria-hidden="true"></span>
				{/if}
			</Button>
		{/each}
	</div>
	{#if error}
		<Alert.Root variant="destructive">
			<Alert.Title>Không thể đổi vòng thi</Alert.Title>
			<Alert.Description>{error}</Alert.Description>
		</Alert.Root>
	{/if}
</nav>
