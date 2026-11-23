/** Contestant slot (1..4) of a VĐ pre_ques_tsN / ques_tsN screen, 0 otherwise. */
export function screenSlot(screen: string): number {
	return Number(/^(?:pre_ques|ques)_ts([1-4])$/.exec(screen)?.[1] ?? 0);
}

/** Tier key ('20' | '30') of a package choice such as '20_1'. */
export function choiceTier(choice: string | undefined): string {
	return choice?.split('_')[0] ?? '';
}

/** Legacy tier label: the configured tier value (defaults 20/30). */
export function tierLabel(tiers: readonly number[] | undefined, tier: string): number | string {
	if (tier !== '20' && tier !== '30') return '';
	const index = tier === '30' ? 1 : 0;
	return tiers?.[index] ?? (index ? 30 : 20);
}
