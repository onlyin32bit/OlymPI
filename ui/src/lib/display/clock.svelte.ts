import { socket } from '$lib/socket';

export type ServerClock = {
	/** Server time in ms (local clock + socket offset), refreshed every animation frame. */
	readonly now: number;
};

/**
 * Call during component initialisation: `const clock = serverClock();` then read `clock.now`
 * in markup/$derived. Ticks with requestAnimationFrame until the component unmounts.
 */
export function serverClock(): ServerClock {
	let now = $state(socket.serverNow);
	$effect(() => {
		let frame = requestAnimationFrame(function tick() {
			now = socket.serverNow;
			frame = requestAnimationFrame(tick);
		});
		return () => cancelAnimationFrame(frame);
	});
	return {
		get now() {
			return now;
		}
	};
}
