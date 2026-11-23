/** The speaker chosen on this TV ("Chọn loa"); '' means the browser default. */
let sinkId = '';
const listeners = new Set<(sinkId: string) => void>();

if (typeof window !== 'undefined') {
	// Dispatched by AudioEngine.chooseOutput().
	window.addEventListener('4t:audio-output', (event) => {
		sinkId = String((event as CustomEvent<string>).detail ?? '');
		for (const listener of listeners) listener(sinkId);
	});
}

/** Calls the listener now and whenever the chosen speaker changes. */
export function onChosenSinkId(listener: (sinkId: string) => void): () => void {
	listeners.add(listener);
	listener(sinkId);
	return () => listeners.delete(listener);
}
