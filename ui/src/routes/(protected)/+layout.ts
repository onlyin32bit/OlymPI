import type { LayoutLoad } from './$types';
import { api } from '$lib/api';
import { redirect } from '@sveltejs/kit';

export const load: LayoutLoad = async ({ fetch }) => {
	const user = await api.auth.refresh(fetch);
	if (!user) redirect(307, '/#/');
	return { user };
};
