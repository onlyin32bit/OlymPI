<script lang="ts">
	import RichText from '$lib/components/common/rich-text.svelte';
	import { assetURL, type DisplayContext } from '$lib/display/types';
	import { legacyTextSize } from './legacy-text-size';

	let { context }: { context: DisplayContext } = $props();
	let question = $derived(context.match?.current_question);
	let content = $derived(question?.content ?? '');
</script>

<div class="fixed h-full w-full bg-bg-2 bg-cover bg-no-repeat">
	<div>
		<img
			class="absolute left-[10cqw] w-[73cqw]"
			style="filter: drop-shadow(8px 28px 32px #335);"
			src="/bg-tt.svg"
			alt="BG"
		/>
		<div
			class="fixed bottom-[3.2cqh] left-[5cqw] h-[92.5cqh] w-[6cqw] overflow-hidden bg-gray-200 outline-8"
			style="filter: drop-shadow(8px 28px 32px #335); border: 1cqh solid #88badf;"
		>
			<div class="absolute top-0 h-[110%] w-full" style="background: #f00"></div>
		</div>
		<div class="fixed left-[14cqw] top-[7cqh] h-[12cqh] w-[64cqw] font-semibold">
			<div
				class="absolute left-1/2 top-1/2 z-50 w-[63cqw] -translate-x-1/2 -translate-y-1/2 text-center"
				style="font-size: 3cqh; text-shadow: 3px 1px 11px rgba(103, 103, 140, 1);"
				use:legacyTextSize={{ key: content, step: 0.2 }}
			>
				<div class="whitespace-pre-line">
					<RichText value={content} />
				</div>
			</div>
		</div>
		<div class="fixed bottom-[4.42cqh] left-[14cqw] h-[72cqh] w-[65cqw]">
			{#if question?.explanation_asset_id}
				<img
					class="absolute left-1/2 h-full -translate-x-1/2"
					src={assetURL(question.explanation_asset_id)}
					alt=""
				/>
			{/if}
		</div>
	</div>
</div>
