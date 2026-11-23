import type { Sound, SoundUpdate } from '$lib/types';
import type { HttpClient } from '../client';

type SoundsResponse = { sounds: Sound[] | null };
type SoundResponse = { sound: Sound };

export class SoundsApi {
	constructor(private readonly http: HttpClient) {}

	async list(): Promise<Sound[]> {
		return (await this.http.request<SoundsResponse>('/sounds')).sounds ?? [];
	}

	async manage(): Promise<Sound[]> {
		return (await this.http.request<SoundsResponse>('/sounds/manage')).sounds ?? [];
	}

	async upload(file: File, metadata: SoundUpdate): Promise<Sound> {
		const form = new FormData();
		form.append('file', file);
		form.append('metadata', JSON.stringify(metadata));
		return (
			await this.http.request<SoundResponse>('/sounds', {
				method: 'POST',
				body: form
			})
		).sound;
	}

	async update(id: string, metadata: SoundUpdate): Promise<Sound> {
		return (
			await this.http.request<SoundResponse>(`/sounds/${id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(metadata)
			})
		).sound;
	}

	async archive(id: string): Promise<void> {
		await this.http.request(`/sounds/${id}`, { method: 'DELETE' });
	}
}
