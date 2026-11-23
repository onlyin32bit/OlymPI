import type { HttpClient } from '../client';
import type { VCNVView } from '$lib/display/types';

export type VCNVDisplay = VCNVView;

export class PublicApi {
	constructor(private readonly http: HttpClient) {}

	async vcnv(options: Pick<RequestInit, 'signal'> = {}): Promise<VCNVDisplay> {
		return (await this.http.request<{ vcnv: VCNVDisplay }>('/match/current/vcnv', options)).vcnv;
	}
}
