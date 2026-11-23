import type { User } from '$lib/types';
import { SvelteURLSearchParams } from 'svelte/reactivity';
import { ApiError, type Fetch, type HttpClient } from '../client';

type UserResponse = { user: User };

export class AuthApi {
	#user = $state<User | null>(null);
	#revision = 0;

	constructor(
		private readonly http: HttpClient,
		private readonly sessionChanged: () => void,
		private readonly sessionInvalidated?: () => void
	) {}

	get user(): User | null {
		return this.#user;
	}

	async login(username: string, password: string): Promise<User> {
		await this.http.request('/login', {
			method: 'POST',
			headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
			body: new SvelteURLSearchParams({ username, password })
		});
		const user = await this.refresh();
		if (!user) throw new Error('The server did not establish an authenticated session');
		this.sessionChanged();
		return user;
	}

	async logout(): Promise<void> {
		try {
			await this.http.request('/logout', { method: 'POST' });
		} finally {
			this.clear();
		}
	}

	async refresh(fetcher?: Fetch): Promise<User | null> {
		const revision = ++this.#revision;
		try {
			const { user } = await this.http.request<UserResponse>('/me', { fetch: fetcher });
			if (revision === this.#revision) this.#user = user;
		} catch (error) {
			if (!(error instanceof ApiError) || error.status !== 401) throw error;
			if (revision === this.#revision) this.clear();
		}
		return this.#user;
	}

	clear(): void {
		this.#revision++;
		const wasAuthenticated = this.#user !== null;
		this.#user = null;
		if (wasAuthenticated) this.sessionInvalidated?.();
	}
}
