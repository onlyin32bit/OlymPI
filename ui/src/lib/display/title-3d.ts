import type { Font, Path } from 'opentype.js';
import {
	ACESFilmicToneMapping,
	Color,
	DirectionalLight,
	ExtrudeGeometry,
	Group,
	Mesh,
	MeshStandardMaterial,
	PerspectiveCamera,
	PMREMGenerator,
	Scene,
	ShapePath,
	SRGBColorSpace,
	WebGLRenderer,
	type BufferGeometry
} from 'three';
import { RoomEnvironment } from 'three/examples/jsm/environments/RoomEnvironment.js';

export type TitlePalette = 'silver' | 'gold';

const PALETTES: Record<TitlePalette, { color: string; metalness: number; roughness: number; light: string }> = {
	silver: { color: '#d9dee8', metalness: 1, roughness: 0.32, light: '#ffffff' },
	gold: { color: '#e8b832', metalness: 1, roughness: 0.3, light: '#fff1c8' }
};

const FONT_URL = '/fonts/display-title.ttf';
let fontPromise: Promise<Font> | undefined;
function loadFont(): Promise<Font> {
	fontPromise ??= fetch(FONT_URL)
		.then((response) => response.arrayBuffer())
		.then(async (buffer) => {
			// Browser builds expose named exports, Node's CommonJS build only a default.
			const module = (await import('opentype.js')) as typeof import('opentype.js') & { default?: typeof import('opentype.js') };
			return (module.parse ?? module.default!.parse)(buffer);
		});
	return fontPromise;
}

/** Wide lens at the start exaggerates depth; it relaxes to a gentle perspective. */
const FOV_START = 62;
const FOV_END = 20;

const easeOutCubic = (t: number) => 1 - (1 - t) ** 3;
const easeOutQuart = (t: number) => 1 - (1 - t) ** 4;
const clamp01 = (t: number) => Math.min(1, Math.max(0, t));

interface Letter {
	mesh: Mesh;
	home: { x: number; y: number };
	slot: number;
	delay: number;
}

export interface TitleOptions {
	text: string;
	palette?: TitlePalette;
	/** Cap height of the text in canvas pixels (canvas is 1920 wide). */
	fontPx?: number;
	/** Text centre, measured from the canvas top in pixels. */
	centerY?: number;
	duration?: number;
	/** The whole entrance, first letter start to last letter landing, in ms. */
	totalMs?: number;
	/** Lay the title out at its final pose with no entrance animation. */
	still?: boolean;
}

/**
 * Real 3D title: each glyph is an extruded, bevelled mesh lit by an image-based environment.
 * Rendering is on demand: frames are drawn only while the entrance animation runs.
 */
export class Title3D {
	private renderer: WebGLRenderer;
	private scene = new Scene();
	private camera: PerspectiveCamera;
	private letters: Letter[] = [];
	private geometries: BufferGeometry[] = [];
	private material: MeshStandardMaterial;
	private pmrem: PMREMGenerator;
	private envTexture: { dispose(): void };
	private frame = 0;
	private disposed = false;
	private start = 0;
	private total = 0;
	private reducedMotion = false;

	constructor(
		private canvas: HTMLCanvasElement,
		private options: TitleOptions
	) {
		const width = canvas.width, height = canvas.height;
		this.renderer = new WebGLRenderer({ canvas, alpha: true, antialias: true, powerPreference: 'high-performance' });
		this.renderer.setPixelRatio(1);
		this.renderer.setSize(width, height, false);
		this.renderer.outputColorSpace = SRGBColorSpace;
		this.renderer.toneMapping = ACESFilmicToneMapping;
		this.renderer.toneMappingExposure = 1.05;
		this.pmrem = new PMREMGenerator(this.renderer);
		const env = this.pmrem.fromScene(new RoomEnvironment(), 0.03);
		this.envTexture = env;
		this.scene.environment = env.texture;
		const palette = PALETTES[options.palette ?? 'silver'];
		this.material = new MeshStandardMaterial({
			color: new Color(palette.color),
			metalness: palette.metalness,
			roughness: palette.roughness,
			envMapIntensity: 1.25
		});
		const key = new DirectionalLight(palette.light, 1.6);
		key.position.set(-300, 500, 700);
		this.scene.add(key);
		// A 1px = 1 world unit camera at z = 0, with a modest field of view for slight perspective.
		this.camera = new PerspectiveCamera(FOV_END, width / height, 10, 6000);
		this.setFov(FOV_START);
		this.reducedMotion = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
	}

	async init(): Promise<void> {
		const font = await loadFont();
		if (this.disposed) return;
		const { text, fontPx = 130, centerY = 190, duration = 1400, totalMs = 4000 } = this.options;
		const width = this.canvas.width, height = this.canvas.height;
		const unit = fontPx / (font.unitsPerEm * 0.72); // 0.72 ≈ cap height / em
		// '\n' starts a new line; every line is centred and shares one fit scale.
		const lines = text.split('\n').map((line) => [...line]);
		const advancesOf = lines.map((line) => line.map((character) => font.charToGlyph(character).advanceWidth ?? 0));
		const widest = Math.max(...advancesOf.map((row) => row.reduce((sum, value) => sum + value, 0) * unit));
		const fit = Math.min(1, (width * 0.9) / widest);
		const scale = unit * fit;
		const cap = font.unitsPerEm * 0.72 * scale;
		const lineGap = cap * 1.5;
		const group = new Group();
		let index = 0;
		lines.forEach((line, row) => {
			const lineWidth = advancesOf[row].reduce((sum, value) => sum + value, 0) * scale;
			let cursor = -lineWidth / 2;
			const centre = height / 2 - centerY - (row - (lines.length - 1) / 2) * lineGap;
			const baseY = centre - cap / 2;
			line.forEach((character, column) => {
				const advance = advancesOf[row][column] * scale;
				const glyph = font.charToGlyph(character);
				if (character !== ' ' && glyph.path.commands.length) {
					const geometry = this.buildGeometry(glyph.path, scale);
					geometry.computeBoundingBox();
					const box = geometry.boundingBox!;
					const cx = (box.min.x + box.max.x) / 2, cy = (box.min.y + box.max.y) / 2;
					geometry.translate(-cx, -cy, 0);
					const mesh = new Mesh(geometry, this.material);
					const home = { x: cursor + cx, y: baseY + cy };
					mesh.position.set(home.x, home.y, 0);
					mesh.visible = false;
					group.add(mesh);
					this.geometries.push(geometry);
					this.letters.push({ mesh, home, slot: index, delay: 0 });
				}
				cursor += advance;
				index++;
			});
		});
		// Spread the letter starts so the last letter lands exactly at totalMs.
		const gaps = Math.max(1, index - 1);
		const stagger = Math.max(0, (totalMs - duration) / gaps);
		for (const letter of this.letters) letter.delay = letter.slot * stagger;
		this.scene.add(group);
		this.total = Math.max(totalMs, duration);
		this.duration = duration;
		this.start = performance.now();
		if (this.reducedMotion || this.options.still) {
			this.pose(Infinity);
			this.renderer.render(this.scene, this.camera);
			return;
		}
		this.frame = requestAnimationFrame(this.tick);
	}

	private duration = 1500;

	private buildGeometry(path: Path, scale: number): ExtrudeGeometry {
		const shapePath = new ShapePath();
		for (const c of path.commands) {
			// Font units are y-up, matching three.js.
			if (c.type === 'M') shapePath.moveTo(c.x * scale, c.y * scale);
			else if (c.type === 'L') shapePath.lineTo(c.x * scale, c.y * scale);
			else if (c.type === 'Q') shapePath.quadraticCurveTo(c.x1 * scale, c.y1 * scale, c.x * scale, c.y * scale);
			else if (c.type === 'C') shapePath.bezierCurveTo(c.x1 * scale, c.y1 * scale, c.x2 * scale, c.y2 * scale, c.x * scale, c.y * scale);
			else if (c.type === 'Z') shapePath.currentPath?.closePath();
		}
		const depth = this.options.fontPx ? this.options.fontPx * 0.2 : 30;
		return new ExtrudeGeometry(shapePath.toShapes(), {
			depth,
			bevelEnabled: true,
			bevelThickness: depth * 0.14,
			bevelSize: depth * 0.08,
			bevelSegments: 3,
			curveSegments: 6
		});
	}

	/**
	 * Positions every letter for `elapsed` ms after the start. Each letter climbs from below the
	 * frame, close to the camera and left of its slot, standing edge-on and turning about its
	 * vertical axis until it faces the viewer. There is no overshoot and no tumbling.
	 */
	private pose(elapsed: number): void {
		const height = this.canvas.height;
		const progress = this.total > 0 ? clamp01(elapsed / this.total) : 1;
		this.setFov(FOV_START + (FOV_END - FOV_START) * easeOutCubic(progress));
		for (const letter of this.letters) {
			const t = clamp01((elapsed - letter.delay) / this.duration);
			const { mesh, home } = letter;
			if (t <= 0) {
				mesh.visible = false;
				continue;
			}
			mesh.visible = true;
			const left = 1 - easeOutCubic(t);
			const turn = 1 - easeOutQuart(t);
			mesh.position.x = home.x - left * 150;
			mesh.position.y = home.y - left * height * 0.85;
			mesh.position.z = left * 320;
			mesh.rotation.x = 0;
			mesh.rotation.y = -turn * 1.65;
			mesh.rotation.z = left * 0.06;
		}
	}

	private tick = (now: number): void => {
		if (this.disposed) return;
		const elapsed = now - this.start;
		this.pose(elapsed);
		this.renderer.render(this.scene, this.camera);
		if (elapsed < this.total) this.frame = requestAnimationFrame(this.tick);
	};

	/** Dolly-zoom: the z = 0 plane keeps the same framing while the field of view changes. */
	private setFov(fov: number): void {
		this.camera.fov = fov;
		this.camera.position.z = this.canvas.height / 2 / Math.tan((fov * Math.PI) / 360);
		this.camera.updateProjectionMatrix();
	}

	/** Preview helper: freezes the entrance at `ms` after the start. */
	seek(ms: number): void {
		cancelAnimationFrame(this.frame);
		this.pose(ms);
		this.renderer.render(this.scene, this.camera);
	}

	get length(): number {
		return this.total;
	}

	dispose(): void {
		this.disposed = true;
		cancelAnimationFrame(this.frame);
		for (const geometry of this.geometries) geometry.dispose();
		this.material.dispose();
		this.envTexture.dispose();
		this.pmrem.dispose();
		this.renderer.dispose();
		this.renderer.forceContextLoss();
	}
}
