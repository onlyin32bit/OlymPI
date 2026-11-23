import type { ActionReturn } from 'svelte/action';

export type LegacyTextSizeOptions = {
	/** Re-run key (question content). */
	key: string;
	/** Legacy growth step in cqh (ques 0.1, solve 0.2). */
	step: number;
};

const BASE_CQH = 3;
const GROW_UNTIL = 0.85;
const MAX_STEPS = 400;

/**
 * Legacy TT header sizing: start at 3vh(=cqh) and grow by `step` while the text fills less than
 * 85% of the 12cqh container. Below the legacy minimum only when the text would overflow its
 * room (centred at 13cqh from the top, 10.58cqh above the media area → ≤ 21cqh tall).
 * Measures synchronously so the text never flashes at the wrong size.
 */
export function legacyTextSize(
	text: HTMLElement,
	options: LegacyTextSizeOptions
): ActionReturn<LegacyTextSizeOptions> {
	let current = options;
	let disposed = false;

	function fit(): void {
		if (disposed || !text.isConnected) return;
		const container = text.parentElement;
		if (!container) return;
		const containerHeight = container.clientHeight;
		if (containerHeight <= 0) return;
		const cqh = containerHeight / 12;
		const maxHeight = 21 * cqh;
		let size = BASE_CQH;
		const apply = () => (text.style.fontSize = `${size}cqh`);
		apply();
		let steps = 0;
		if (text.offsetHeight > maxHeight) {
			while (text.offsetHeight > maxHeight && size > 0.5 && steps++ < MAX_STEPS) {
				size = Math.round((size - 0.1) * 100) / 100;
				apply();
			}
			return;
		}
		while (text.offsetHeight / containerHeight < GROW_UNTIL && steps++ < MAX_STEPS) {
			size = Math.round((size + current.step) * 100) / 100;
			apply();
			if (text.offsetHeight > maxHeight) {
				size = Math.round((size - current.step) * 100) / 100;
				apply();
				break;
			}
		}
	}

	fit();
	queueMicrotask(fit);
	void text.ownerDocument.fonts?.ready.then(fit);

	return {
		update(next) {
			const changed = next.key !== current.key;
			current = next;
			if (changed) {
				fit();
				queueMicrotask(fit);
			}
		},
		destroy() {
			disposed = true;
		}
	};
}
