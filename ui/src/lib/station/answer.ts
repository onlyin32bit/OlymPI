/** The text a station sends: one accent encoding (NFC), single spaces, capitals. */
export function normalizeAnswer(raw: string): string {
	return raw.normalize('NFC').replace(/\s+/g, ' ').trim().toLocaleUpperCase('vi');
}

const FINAL = '(?:NG|NH|CH|C|M|N|P|T)?';
/** Telex patterns that never survive in a correctly converted Vietnamese syllable. */
const RAW_TELEX = [
	// dd → đ at the start of a word (DDUWOWNGF for ĐƯỜNG).
	/^DD[A-Z]/,
	// A doubled or w-marked vowel followed by a tone key (TROONGS for TRỐNG, VIEETJ for VIỆT).
	new RegExp(`^[^AEIOUY]*[AEIOUY]*(?:AA|EE|OO|AW|OW|UW)[AEIOUYW]*${FINAL}[SFRXJ]$`),
	// A Vietnamese rhyme ending in the f/j tone keys (HOAF for HOÀ).
	new RegExp(`^[^AEIOUY]*[AEIOUY]+${FINAL}[FJ]$`)
];

/**
 * Non-blocking hint that the answer was typed with the Vietnamese input method
 * switched off. English words rarely match these syllable shapes.
 */
export function looksLikeRawTelex(raw: string): boolean {
	const text = normalizeAnswer(raw);
	if (!text || /[^\x20-\x7E]/.test(text)) return false;
	return text
		.split(' ')
		.some((word) => /^[A-Z]{3,}$/.test(word) && RAW_TELEX.some((pattern) => pattern.test(word)));
}
