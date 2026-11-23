declare module 'opentype.js' {
	export interface PathCommand {
		type: 'M' | 'L' | 'Q' | 'C' | 'Z';
		x: number;
		y: number;
		x1: number;
		y1: number;
		x2: number;
		y2: number;
	}
	export interface Path {
		commands: PathCommand[];
	}
	export interface Glyph {
		advanceWidth?: number;
		path: Path;
	}
	export interface Font {
		unitsPerEm: number;
		charToGlyph(character: string): Glyph;
	}
	export function parse(buffer: ArrayBuffer): Font;
}
