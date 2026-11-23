import type { SystemSettings } from '$lib/types';
import type { HttpClient } from './client';

type SetupStatus = {
	configured: boolean;
	needs_admin: boolean;
	system?: SystemSettings;
};

type SetupRequest = {
	username: string;
	password: string;
	bind: string;
	port: number;
	secure_cookies: boolean;
};

type SetupResponse = { setup_complete: boolean; restart_required: boolean };

export class SetupApi {
	constructor(private readonly http: HttpClient) {}

	async status(): Promise<SetupStatus> {
		return this.http.request<SetupStatus>('/setup/status');
	}

	async complete(values: SetupRequest): Promise<SetupResponse> {
		return this.http.request<SetupResponse>('/setup', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(values)
		});
	}
}

export type { SetupRequest, SetupResponse, SetupStatus };
