import type { SystemSettings } from '$lib/types';
import type { HttpClient } from '../client';

type SystemResponse = { system: SystemSettings; restart_required?: boolean };

export class SystemApi {
	#current = $state<SystemSettings | null>(null);

	constructor(private readonly http: HttpClient) {}

	get current(): SystemSettings | null {
		return this.#current;
	}

	async get(): Promise<SystemSettings> {
		const { system } = await this.http.request<SystemResponse>('/system-settings');
		this.#current = system;
		return system;
	}

	async update(system: SystemSettings): Promise<SystemResponse> {
		const response = await this.http.request<SystemResponse>('/system-settings', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(system)
		});
		this.#current = response.system;
		return response;
	}
}
