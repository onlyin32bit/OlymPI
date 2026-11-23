import katex from 'katex';

const escapeHTML = (value: string) =>
	value.replace(
		/[&<>"']/g,
		(character) =>
			({
				'&': '&amp;',
				'<': '&lt;',
				'>': '&gt;',
				'"': '&quot;',
				"'": '&#39;'
			})[character] ?? character
	);

export function renderRichText(source: string): string {
	const math: string[] = [];
	const withTokens = source.replace(/\$\$([\s\S]+?)\$\$|\$([^\n$]+?)\$/g, (_, block, inline) => {
		const expression = block ?? inline;
		const token = `RICHMATHTOKEN${math.length}END`;
		try {
			math.push(
				katex.renderToString(expression.trim(), {
					displayMode: block !== undefined,
					throwOnError: false,
					strict: false,
					trust: false
				})
			);
		} catch {
			math.push(escapeHTML(block !== undefined ? `$$${expression}$$` : `$${expression}$`));
		}
		return token;
	});

	let html = escapeHTML(withTokens.replace(/\r\n?/g, '\n'));
	html = html
		.replace(/\*\*([^\n]+?)\*\*/g, '<strong>$1</strong>')
		.replace(/__([^\n]+?)__/g, '<u>$1</u>')
		.replace(/~~([^\n]+?)~~/g, '<s>$1</s>')
		.replace(/(^|[^*])\*([^*\n]+?)\*(?!\*)/g, '$1<em>$2</em>')
		.replace(/\n/g, '<br />');

	return html.replace(/RICHMATHTOKEN(\d+)END/g, (_, index) => math[Number(index)] ?? '');
}
