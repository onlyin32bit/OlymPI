import type { SiteSettings } from '$lib/types';
import type { HttpClient } from '../client';

type SettingsResponse = { settings: SiteSettings };

export type SettingsLoadOptions = {
	signal?: AbortSignal;
	/** First retry delay; doubles up to maxDelayMs. */
	initialDelayMs?: number;
	maxDelayMs?: number;
};

export class SettingsApi {
	#current = $state<SiteSettings | null>(null);
	// Only the newest read or write may set current; late responses are dropped.
	#sequence = 0;

	constructor(private readonly http: HttpClient) {}

	get current(): SiteSettings | null {
		return this.#current;
	}

	async get(): Promise<SiteSettings> {
		const sequence = ++this.#sequence;
		const { settings } = await this.http.request<SettingsResponse>('/settings');
		if (sequence === this.#sequence) this.#current = settings;
		return settings;
	}

	/**
	 * Fetches settings, retrying with backoff until it succeeds or the signal
	 * aborts (rejects with AbortError). For screens that cannot work without
	 * settings, such as the audience display.
	 */
	async load(options: SettingsLoadOptions = {}): Promise<SiteSettings> {
		const { signal, initialDelayMs = 500, maxDelayMs = 5_000 } = options;
		let delay = initialDelayMs;
		for (;;) {
			signal?.throwIfAborted();
			try {
				return await this.get();
			} catch (cause) {
				signal?.throwIfAborted();
				console.warn('Unable to load settings; retrying', cause);
			}
			const { promise, resolve, reject } = Promise.withResolvers<void>();
			const abort = () => {
				clearTimeout(timer);
				reject(signal?.reason);
			};
			const timer = setTimeout(() => {
				signal?.removeEventListener('abort', abort);
				resolve();
			}, delay);
			signal?.addEventListener('abort', abort, { once: true });
			await promise;
			delay = Math.min(maxDelayMs, delay * 2);
		}
	}

	async update(settings: SiteSettings): Promise<SiteSettings> {
		const sequence = ++this.#sequence;
		const response = await this.http.request<SettingsResponse>('/settings', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(settings)
		});
		if (sequence === this.#sequence) this.#current = response.settings;
		return response.settings;
	}
}
