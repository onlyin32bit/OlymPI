import type { ActionReturn } from 'svelte/action';

/**
 * Legacy ScreenQuestionVD text sizing as a deterministic measure loop: start at 6vh, grow 0.2vh
 * while the text is under 50% of the question box, shrink 0.5vh while it is over 80%; then
 * shrink only if the text would still overflow its width. Units are container units (cqh).
 * `container` is the 65cqh question box; re-runs when `key` (question/content) changes.
 */
export function legacyTextSize(
	node: HTMLElement,
	params: { container: HTMLElement | undefined; key: string }
): ActionReturn<{ container: HTMLElement | undefined; key: string }> {
	let current = params;
	let frame: number | undefined;
	let disposed = false;

	function fit(): void {
		frame = undefined;
		const box = current.container;
		if (disposed || !box || !node.isConnected) return;
		const boxHeight = box.clientHeight;
		if (boxHeight <= 0) return;
		let size = 6;
		const apply = () => (node.style.fontSize = `${size}cqh`);
		apply();
		for (let step = 0; step < 400; step++) {
			const percent = (node.offsetHeight / boxHeight) * 100;
			if (percent < 50) size += 0.2;
			else if (percent > 80) size -= 0.5;
			else break;
			if (size <= 0.5) {
				size = 0.5;
				apply();
				break;
			}
			apply();
		}
		while (size > 0.5 && node.scrollWidth > node.clientWidth + 1) {
			size -= 0.5;
			apply();
		}
	}

	function schedule(): void {
		if (!disposed && frame === undefined) frame = requestAnimationFrame(fit);
	}

	const fonts = node.ownerDocument.fonts;
	void fonts?.ready.then(schedule, schedule);
	fonts?.addEventListener('loadingdone', schedule);
	schedule();
	return {
		update(next) {
			const changed = next.key !== current.key || next.container !== current.container;
			current = next;
			if (changed) schedule();
		},
		destroy() {
			disposed = true;
			if (frame !== undefined) cancelAnimationFrame(frame);
			fonts?.removeEventListener('loadingdone', schedule);
		}
	};
}
