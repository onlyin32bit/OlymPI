<script lang="ts">
	import { api } from '$lib/api';
	import ScoringRulesForm from '$lib/components/operator/scoring-rules-form.svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { DEFAULT_SCORING } from '$lib/operator/scoring';
	import type { ScoringRules } from '$lib/types';
	import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';

	let open = $state(false);
	let saving = $state(false);
	let error = $state('');

	async function save(scoring: ScoringRules): Promise<boolean> {
		const current = api.settings.current ?? (await api.settings.get());
		saving = true;
		error = '';
		try {
			await api.settings.update({ ...current, scoring });
			open = false;
			return true;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Không thể lưu luật điểm';
			return false;
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Trigger class="cp-btn is-ghost rules-trigger"><SlidersHorizontal />Luật điểm</Dialog.Trigger>
	<Dialog.Content class="max-h-[90vh] overflow-y-auto sm:max-w-4xl">
		<Dialog.Header>
			<Dialog.Title>Luật tính điểm</Dialog.Title>
			<Dialog.Description>Điểm gợi ý khi bấm Đúng/Sai. Thay đổi không làm đổi điểm đã ghi.</Dialog.Description>
		</Dialog.Header>
		{#if error}<p class="text-sm text-destructive" role="alert">{error}</p>{/if}
		<ScoringRulesForm value={api.settings.current?.scoring ?? DEFAULT_SCORING} {saving} onsave={save} dense />
	</Dialog.Content>
</Dialog.Root>

<style>
	:global(.rules-trigger) {
		--cp-h: 32px;
		padding: 0 10px;
		color: var(--cp-muted);
		font-size: 13px;
	}
</style>
