import { mount } from 'svelte';
import '../../src/app.css';
import Display from './text-fit-display.svelte';
import { fixture } from './controller-state.svelte';
import type { Phases } from '../../src/lib/types';

const longQuestion =
	'Nhà thám hiểm nổi tiếng James Cook - người đã thực hiện ba chuyến hải trình đến Thái Bình Dương đã ghi chép trong nhật ký của mình về một lục địa trên Trái Đất như sau: "Tôi tin chắc điều này và chúng ta, còn hơn là có thể, đã nhìn thấy một phần của nó". Đó là lục địa nào?';
function question(phase: Phases, content: string, visible = true) {
	fixture.reset(
		phase,
		phase === 'khoi_dong' ? 'ques_chung' : phase === 've_dich' ? 'ques_ts1' : 'ques'
	);
	fixture.setQuestion(
		phase === 'khoi_dong'
			? 'kd_chung.4'
			: phase === 'vcnv'
				? 'vcnv.rows.0'
				: phase === 'tang_toc'
					? 'tt.0'
					: 'vd.0.20.0'
	);
	const runtime = fixture.runtime;
	if (!runtime.current_question) throw new Error('Missing fixture question');
	runtime.current_question.content = content;
	runtime.current_question.type = 'text';
	runtime.current_question.asset_id = '';
	runtime.controller.selected_contestant = phase === 've_dich' ? 1 : 0;
	runtime.controller.is_showing_question = visible;
}
question('khoi_dong', longQuestion);
const target = document.getElementById('display-root');
if (!target) throw new Error('Missing display test root');
mount(Display, { target });
Object.assign(window, {
	textFitQA: {
		longQuestion,
		question,
		get runtime() {
			return fixture.runtime;
		},
		updateText(content: string) {
			if (fixture.runtime.current_question) fixture.runtime.current_question.content = content;
		},
		tick() {
			fixture.runtime.version++;
			fixture.runtime.timer.remaining_ms -= 100;
		}
	}
});
