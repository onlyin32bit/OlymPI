import { sineOut } from 'svelte/easing';
import type { TransitionConfig } from 'svelte/transition';

/** Legacy `typewriter`: the node must hold a single text node. */
export function typewriter(
	node: Element,
	{ delay = 0, speed = 1 }: { delay?: number; speed?: number } = {}
): TransitionConfig {
	const valid = node.childNodes.length === 1 && node.childNodes[0].nodeType === Node.TEXT_NODE;
	if (!valid) throw new Error('This transition only works on elements with a single text node child');
	const text = node.textContent ?? '';
	const duration = text.length / (speed * 0.01);
	return {
		delay,
		duration,
		tick: (t: number) => {
			node.textContent = text.slice(0, Math.trunc(text.length * t));
		}
	};
}

/** Legacy `zoomIn` (3D letter fly-in of ScreenStart); vh became cqh on the display canvas. */
export function zoomIn(
	_node: Element,
	{
		duration = 1000,
		delay = 0,
		easing = sineOut
	}: { duration?: number; delay?: number; easing?: (t: number) => number } = {}
): TransitionConfig {
	return {
		duration,
		delay,
		css: (t: number) => {
			const eased = easing(t);
			return `transform: scale(${3 - eased * 2}) translate(-${9 - eased * 9}cqh, ${20 - eased * 20}cqh) rotate3d(0,-1,0,${0.4 - eased * 0.4}turn)`;
		}
	};
}
