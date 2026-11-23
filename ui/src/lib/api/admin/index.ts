import type { Asset, AuditEntry, AuthSession, ContestantAccount, User } from '$lib/types';
import type { HttpClient } from '../client';

export class AuditApi {
	constructor(private readonly http: HttpClient) {}

	async list(query = '', limit?: number): Promise<AuditEntry[]> {
		const params = new URLSearchParams();
		if (query) params.set('q', query);
		if (limit) params.set('limit', String(limit));
		const suffix = params.size ? `?${params}` : '';
		return (
			(await this.http.request<{ audit_logs: AuditEntry[] | null }>(`/audit-logs${suffix}`))
				.audit_logs ?? []
		);
	}
}

export class ContestantAccountsApi {
	constructor(private readonly http: HttpClient) {}

	async list(): Promise<ContestantAccount[]> {
		return (
			(await this.http.request<{ accounts: ContestantAccount[] | null }>('/contestant-accounts'))
				.accounts ?? []
		);
	}

	async resetPassword(slot: number, password: string): Promise<void> {
		await this.http.request(`/contestant-accounts/${slot}/password`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ password })
		});
	}
}

export class AdminAccountsApi {
	constructor(private readonly http: HttpClient) {}

	async list(): Promise<User[]> {
		return (await this.http.request<{ accounts: User[] | null }>('/admin-accounts')).accounts ?? [];
	}

	async create(username: string, password: string): Promise<User> {
		return (
			await this.http.request<{ account: User }>('/admin-accounts', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ username, password })
			})
		).account;
	}

	async resetPassword(id: number, password: string): Promise<void> {
		await this.http.request(`/admin-accounts/${id}/password`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ password })
		});
	}

	async setEnabled(id: number, enabled: boolean): Promise<void> {
		await this.http.request(`/admin-accounts/${id}/enabled`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ enabled })
		});
	}
}

export class SessionsApi {
	constructor(private readonly http: HttpClient) {}

	async list(): Promise<AuthSession[]> {
		return (
			(await this.http.request<{ sessions: AuthSession[] | null }>('/sessions')).sessions ?? []
		);
	}

	async invalidate(id: string): Promise<void> {
		await this.http.request(`/sessions/${id}`, { method: 'DELETE' });
	}
}

export class AssetsApi {
	constructor(private readonly http: HttpClient) {}

	async upload(file: File): Promise<Asset> {
		const form = new FormData();
		form.append('file', file);
		return (await this.http.request<{ asset: Asset }>('/assets', { method: 'POST', body: form }))
			.asset;
	}
}

export class AdminApi {
	readonly audit: AuditApi;
	readonly contestants: ContestantAccountsApi;
	readonly accounts: AdminAccountsApi;
	readonly sessions: SessionsApi;
	readonly assets: AssetsApi;

	constructor(http: HttpClient) {
		this.audit = new AuditApi(http);
		this.contestants = new ContestantAccountsApi(http);
		this.accounts = new AdminAccountsApi(http);
		this.sessions = new SessionsApi(http);
		this.assets = new AssetsApi(http);
	}
}
