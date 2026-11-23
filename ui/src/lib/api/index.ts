import { browser } from '$app/environment';
import { page } from '$app/state';
import { socket } from '$lib/socket';
import { AdminApi } from './admin';
import { AuthApi } from './auth/index.svelte';
import { HttpClient } from './client';
import { MatchesApi } from './match';
import { QuestionSetsApi } from './question-set';
import { PublicApi } from './public';
import { SettingsApi } from './settings/index.svelte';
import { SetupApi } from './setup';
import { SystemApi } from './system/index.svelte';
import { SoundsApi } from './sound';

const http = new HttpClient();
const auth = new AuthApi(
	http,
	() => socket.reconnect(),
	() => {
		socket.disconnect();
		// Protected layouts redirect before opening a new anonymous connection.
		if (browser && !page.route.id?.includes('(protected)')) socket.reconnect();
	}
);
http.onUnauthorized = () => auth.clear();
socket.onSessionCheck(() => {
	if (auth.user) void auth.refresh().catch(() => undefined);
});

export const api = {
	auth,
	settings: new SettingsApi(http),
	system: new SystemApi(http),
	setup: new SetupApi(http),
	matches: new MatchesApi(http),
	questionSets: new QuestionSetsApi(http),
	sounds: new SoundsApi(http),
	admin: new AdminApi(http),
	public: new PublicApi(http)
} as const;

export { ApiError } from './client';

if (browser) {
	void api.auth.refresh().catch(() => undefined);
	void api.settings.get().catch(() => undefined);
}
