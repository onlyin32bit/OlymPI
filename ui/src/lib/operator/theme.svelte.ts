export type PanelTheme = 'light' | 'dark';

const STORAGE_KEY = '4t.control-panel.theme';
/** Scopes the panel's neutral tokens to the document so portaled popovers inherit them. */
const ROOT_CLASS = 'cp-next';

export class PanelThemeState {
	mode = $state<PanelTheme>('light');

	/** Applies the remembered theme (or the system one on first launch); returns the cleanup. */
	attach(): () => void {
		const stored = localStorage.getItem(STORAGE_KEY);
		this.mode =
			stored === 'dark' || stored === 'light'
				? stored
				: window.matchMedia('(prefers-color-scheme: dark)').matches
					? 'dark'
					: 'light';
		const root = document.documentElement;
		root.classList.add(ROOT_CLASS);
		root.classList.toggle('dark', this.mode === 'dark');
		return () => root.classList.remove(ROOT_CLASS, 'dark');
	}

	toggle(): void {
		this.mode = this.mode === 'dark' ? 'light' : 'dark';
		localStorage.setItem(STORAGE_KEY, this.mode);
		document.documentElement.classList.toggle('dark', this.mode === 'dark');
	}
}
