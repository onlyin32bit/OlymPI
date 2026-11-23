import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const server = await createServer({
	configFile: false,
	resolve: { alias: { $lib: fileURLToPath(new URL('../src/lib', import.meta.url)) } },
	server: { middlewareMode: true, hmr: false, watch: null }
});
after(() => server.close());
const { ruleText } = await server.ssrLoadModule('/src/lib/display/rule-text.ts');
const { DEFAULT_SCORING } = await server.ssrLoadModule('/src/lib/operator/scoring.ts');
const legacy = JSON.parse(readFileSync(new URL('./fixtures/legacy-rules.json', import.meta.url), 'utf8'));

test('default scoring reproduces the legacy rule text verbatim', () => {
	for (const [phase, content] of Object.entries(legacy)) {
		assert.equal(ruleText(phase, DEFAULT_SCORING).content, content, phase);
		assert.equal(ruleText(phase).content, content, phase);
	}
	assert.equal(ruleText('vcnv').name, 'VƯỢT CHƯỚNG NGẠI VẬT');
	assert.equal(ruleText('khoi_dong').size, 4.3);
});

test('configured point values replace the legacy numbers', () => {
	const s = structuredClone(DEFAULT_SCORING);
	s.khoi_dong = { ...s.khoi_dong, individual_correct: 15, individual_wrong: -2, common_correct: 12, common_wrong: 0 };
	s.tang_toc.ranks = [50, 35, 25, 15];
	s.vcnv = { ...s.vcnv, row_correct: 11, obstacle_by_rows: [80, 70, 60, 50], obstacle_after_center: 25 };
	s.ve_dich = { ...s.ve_dich, tiers: [25, 35], steal_correct_percent: 50, steal_wrong_percent: 0, star_correct_percent: 300, star_wrong_percent: -50 };
	const kd = ruleText('khoi_dong', s).content;
	assert.match(kd, /đúng được 15 điểm, trả lời sai bị trừ 2 điểm\./);
	assert.match(kd, /đúng được 12 điểm, trả lời sai hoặc bấm chuông mà không có câu trả lời trong 3 giây sẽ không bị trừ điểm\./);
	const tt = ruleText('tang_toc', s).content;
	for (const n of [50, 35, 25, 15]) assert.ok(tt.includes(`được ${n} điểm.`));
	const vc = ruleText('vcnv', s).content;
	assert.match(vc, /thí sinh được 11 điểm/);
	for (const n of [80, 70, 60, 50, 25]) assert.ok(vc.includes(`được ${n} điểm.`));
	const vd = ruleText('ve_dich', s).content;
	assert.match(vd, /2 mức điểm là 25 và 35 điểm/);
	assert.match(vd, /cộng thêm 50% số điểm của câu hỏi từ thí sinh đang thi\. Trả lời sai không bị trừ điểm\./);
	assert.match(vd, /Trả lời đúng được 300% số điểm, trả lời sai bị trừ đi một nửa số điểm của câu hỏi đặt NSHV\./);
});
