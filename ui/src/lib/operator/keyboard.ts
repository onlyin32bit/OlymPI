import type { Slot } from './scoring';

export type ShortcutAction =
	| { type: 'screen'; direction: -1 | 1 }
	| { type: 'question'; direction: -1 | 1 }
	| { type: 'timer' }
	| { type: 'toggle-question' }
	| { type: 'verdict'; slot: Slot; verdict: 'correct' | 'wrong' }
	| { type: 'save' }
	| { type: 'steal' }
	| { type: 'toggle-log' }
	| { type: 'stop-sound' };

export type KeyInput = {
	key: string;
	code: string;
	shiftKey: boolean;
	ctrlKey: boolean;
	metaKey: boolean;
	altKey: boolean;
	repeat: boolean;
};

/** Keys the panel owns; their native button activation must be suppressed. */
export const ACTIVATION_KEYS: Record<string, true> = { Space: true, Enter: true, NumpadEnter: true };

const DIGIT_SLOT: Record<string, Slot> = {
	Digit1: 1,
	Digit2: 2,
	Digit3: 3,
	Digit4: 4,
	Numpad1: 1,
	Numpad2: 2,
	Numpad3: 3,
	Numpad4: 4
};

/** Maps a key press to a panel action. Modifier chords are left to the browser. */
export function resolveShortcut(input: KeyInput): ShortcutAction | null {
	if (input.repeat || input.ctrlKey || input.metaKey || input.altKey) return null;
	const slot = DIGIT_SLOT[input.code];
	if (slot) return { type: 'verdict', slot, verdict: input.shiftKey ? 'wrong' : 'correct' };
	if (input.shiftKey) return null;
	switch (input.code) {
		case 'ArrowLeft':
			return { type: 'screen', direction: -1 };
		case 'ArrowRight':
			return { type: 'screen', direction: 1 };
		case 'ArrowUp':
		case 'PageUp':
			return { type: 'question', direction: -1 };
		case 'ArrowDown':
		case 'PageDown':
			return { type: 'question', direction: 1 };
		case 'Space':
			return { type: 'timer' };
		case 'KeyH':
			return { type: 'toggle-question' };
		case 'Enter':
		case 'NumpadEnter':
			return { type: 'save' };
		case 'KeyB':
			return { type: 'steal' };
		case 'KeyL':
			return { type: 'toggle-log' };
		case 'Escape':
			return { type: 'stop-sound' };
		default:
			return null;
	}
}

/** Typing into a field or working inside a dialog must never trigger a show action. */
export function ignoresShortcuts(target: EventTarget | null): boolean {
	if (!target || typeof (target as Element).closest !== 'function') return false;
	const element = target as HTMLElement;
	if (element.isContentEditable) return true;
	return Boolean(element.closest('input, textarea, select, [contenteditable="true"], [role="dialog"], [role="menu"], [role="listbox"]'));
}

/** Drops a second identical action inside the window, like a GO-button lockout. */
export class DoublePressGuard {
	#last = '';
	#at = -Infinity;

	constructor(private readonly windowMS = 300) {}

	accept(action: ShortcutAction, now: number): boolean {
		const signature = JSON.stringify(action);
		if (signature === this.#last && now - this.#at < this.windowMS) return false;
		this.#last = signature;
		this.#at = now;
		return true;
	}
}
