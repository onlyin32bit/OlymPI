import type { ActionReturn } from 'svelte/action';

/** Largest fitting size, to a quarter logical pixel, in at most eleven probes at 160px. */
export function largestFittingFontSize(maximum: number, fits: (size: number) => boolean): number {
	if (!Number.isFinite(maximum) || maximum < 1) throw new RangeError('Invalid maximum font size');
	if (fits(maximum)) return maximum;
	let low = 4;
	let high = Math.ceil(maximum * 4) - 1;
	let best = 1;
	while (low <= high) {
		const middle = Math.floor((low + high) / 2);
		const size = middle / 4;
		if (fits(size)) {
			best = size;
			low = middle + 1;
		} else {
			high = middle - 1;
		}
	}
	return best;
}

/** Fit a fixed-size box's single block child in untransformed 1920×1080 layout pixels. */
export function fitText(node: HTMLElement, maximum: number): ActionReturn<number> | void {
	const view = node.ownerDocument.defaultView;
	if (!view) return;
	let frame: number | undefined;
	let disposed = false;
	let firstFit = true;
	const originalFont = node.style.fontSize;
	const originalVisibility = node.style.visibility;
	node.style.visibility = 'hidden';

	function fit(): void {
		frame = undefined;
		if (disposed || !node.isConnected) return;
		const content = node.firstElementChild as HTMLElement | null;
		const width = node.clientWidth;
		const height = node.clientHeight - 2;
		if (!content || width <= 0 || height <= 0) return;
		const size = largestFittingFontSize(maximum, (candidate) => {
			node.style.fontSize = `${candidate}px`;
			return (
				content.scrollWidth <= width &&
				Math.max(content.scrollHeight, content.offsetHeight) <= height
			);
		});
		node.style.fontSize = `${size}px`;
		if (firstFit) {
			node.style.visibility = originalVisibility;
			firstFit = false;
		}
	}

	function schedule(): void {
		if (!disposed && frame === undefined) frame = view!.requestAnimationFrame(fit);
	}

	// Observe the fixed box, not its measured child: fitting must not resize its own input.
	const resize = view.ResizeObserver ? new view.ResizeObserver(schedule) : null;
	resize?.observe(node);
	if (!resize) view.addEventListener('resize', schedule);
	const mutations = new view.MutationObserver(schedule);
	mutations.observe(node, { childList: true, characterData: true, subtree: true });
	const fonts = node.ownerDocument.fonts;
	void fonts?.ready.then(schedule, schedule);
	fonts?.addEventListener('loadingdone', schedule);
	fonts?.addEventListener('loadingerror', schedule);
	schedule();
	return {
		update(nextMaximum) {
			if (maximum === nextMaximum) return;
			maximum = nextMaximum;
			schedule();
		},
		destroy() {
			disposed = true;
			if (frame !== undefined) view.cancelAnimationFrame(frame);
			resize?.disconnect();
			mutations.disconnect();
			if (!resize) view.removeEventListener('resize', schedule);
			fonts?.removeEventListener('loadingdone', schedule);
			fonts?.removeEventListener('loadingerror', schedule);
			node.style.fontSize = originalFont;
			node.style.visibility = originalVisibility;
		}
	};
}
