import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ parent }) => {
	const { user } = await parent();
	if (user.role !== 'admin') redirect(307, '/#/');
};
