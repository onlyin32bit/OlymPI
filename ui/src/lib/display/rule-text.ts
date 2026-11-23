/**
 * Rule slide text shared by the public display and the contestant station: legacy ScreenRule.svelte
 * text, with the point values taken from the configured scoring (DEFAULT_SCORING == legacy verbatim).
 */
import { DEFAULT_SCORING } from '$lib/operator/scoring';
import type { ScoringRules } from '$lib/types';

export type RuleText = { name: string; content: string; size: number };

/** 'không bị trừ điểm' for 0, otherwise 'bị trừ N điểm' (sign ignored). */
function penalty(points: number): string {
	return points === 0 ? 'không bị trừ điểm' : `bị trừ ${Math.abs(points)} điểm`;
}

/** Legacy VĐ loss phrase (without the leading 'bị trừ đi '). */
function lossOf(percent: number): string {
	const value = Math.abs(percent);
	if (value === 50) return 'một nửa số điểm của câu hỏi';
	if (value === 100) return 'số điểm bằng số điểm của câu hỏi';
	return `${value}% số điểm của câu hỏi`;
}

function lossClause(percent: number, suffix = ''): string {
	return percent === 0 ? 'không bị trừ điểm' : `bị trừ đi ${lossOf(percent)}${suffix}`;
}

function gainOf(percent: number): string {
	if (percent === 200) return 'gấp đôi số điểm';
	return `${percent}% số điểm`;
}

function khoiDong(s: ScoringRules['khoi_dong']): string {
	return `\tPhần thi được chia thành 2 lượt: lượt riêng và lượt chung.
\tTrong lượt riêng, mỗi thí sinh trả lời 10 câu hỏi. Thời gian trả lời cho mỗi câu hỏi là 3 giây từ lúc MC đọc xong câu hỏi. Mỗi câu trả lời đúng được ${s.individual_correct} điểm, trả lời sai ${penalty(s.individual_wrong)}.
\tTrong lượt chung, các thí sinh trả lời 12 câu hỏi trong thời gian không giới hạn. Thí sinh giành quyền trả lời bằng cách bấm chuông. Thí sinh có tối đa 3 giây tính từ lúc giành được quyền trả lời để đưa ra đáp án. Mỗi câu trả lời đúng được ${s.common_correct} điểm, trả lời sai hoặc bấm chuông mà không có câu trả lời trong 3 giây sẽ ${penalty(s.common_wrong)}. Sau 3 giây tính từ thời điểm MC đọc xong câu hỏi, nếu không có thí sinh nào giành quyền trả lời, câu hỏi đó sẽ bị bỏ qua.`;
}

function tangToc(s: ScoringRules['tang_toc']): string {
	const [r1, r2, r3, r4] = s.ranks;
	return `    \tPhần thi sẽ có 4 câu hỏi. Câu 1 và Câu 2 có thời gian trả lời là 20 giây, Câu 3 và Câu 4 có thời gian trả lời là 30 giây. Các thí sinh cùng trả lời bằng máy tính:
• Thí sinh trả lời đúng và nhanh nhất được ${r1} điểm.
• Thí sinh trả lời đúng và nhanh thứ 2 được ${r2} điểm.
• Thí sinh trả lời đúng và nhanh thứ 3 được ${r3} điểm.
• Thí sinh trả lời đúng và nhanh thứ 4 được ${r4} điểm.
\t\tCác câu hỏi trong phần thi sẽ có dạng:
• 1 câu hỏi suy luận, tư duy nhanh.
• 1 câu hỏi dạng IQ-Logic bao gồm các dạng: Tìm số thích hợp điền vào chỗ trống, tìm hình khác nhất, tìm hình còn thiếu, giải mật mã,…
• 1 câu hỏi dạng sắp xếp, ghép nối: bao gồm ghép nối các dữ liệu theo từng cặp hoặc sắp xếp theo một thứ tự nhất định
• 1 câu hỏi dữ kiện: các dữ kiện sẽ lần lượt xuất hiện với độ khó giảm dần. Thí sinh sẽ trả lời các câu hỏi như: Đây là ai? Đây là địa danh nào? Đây là gì?...`;
}

function vcnv(s: ScoringRules['vcnv']): string {
	const [o1, o2, o3, o4] = s.obstacle_by_rows;
	return `  Có 4 từ hàng ngang - cũng chính là 4 gợi ý liên quan đến Chướng ngại vật mà các thí sinh phải đi tìm. Có 1 gợi ý thứ 5 - là 1 hình ảnh liên quan đến CNV hoặc chính là CNV. Hình ảnh được chia thành 5 ô đánh số thứ tự từ 1 đến 4 và một ô trung tâm.
    Mỗi thí sinh có tối đa 1 lượt lựa chọn để chọn trả lời một trong các từ hàng ngang này. Cả 4 thí sinh trả lời câu hỏi bằng máy tính trong thời gian suy nghĩ 15 giây. Trả lời đúng mỗi từ hàng ngang, thí sinh được ${s.row_correct} điểm. Ngoài việc mở được từ hàng ngang nếu trả lời đúng, 1 góc của hình ảnh (được đánh số tương ứng với số từ hàng ngang) cũng được mở ra.
\tThí sinh có thể bấm chuông trả lời CNV bất cứ lúc nào:
\t\t•  Trả lời đúng CNV trong 1 từ hàng ngang đầu tiên  được ${o1} điểm.
\t\t•  Trả lời đúng CNV trong 2 từ hàng ngang được ${o2} điểm.
\t\t•  Trả lời đúng CNV trong 3 từ hàng ngang được ${o3} điểm.
\t\t•  Trả lời đúng CNV trong 4 từ hàng ngang được ${o4} điểm.
\t\t•  Trả lời đúng CNV ở câu hỏi gắn với ô trung tâm được ${s.obstacle_after_center} điểm.
\tNếu trả lời sai CNV thí sinh sẽ bị loại khỏi phần chơi này.`;
}

function veDich(s: ScoringRules['ve_dich']): string {
	const [t1, t2] = s.tiers;
	const stealGain =
		s.steal_correct_percent === 100
			? 'số điểm của câu hỏi'
			: `${s.steal_correct_percent}% số điểm của câu hỏi`;
	return `  Có các câu hỏi ở 2 mức điểm là ${t1} và ${t2} điểm với thời gian trả lời tương ứng là 15 và 20 giây. Trước khi bắt đầu phần thi, thí sinh lựa chọn 3 câu hỏi tùy ý cho mình. Thứ tự tham gia được xác định theo kết quả sau phần thi Tăng tốc và sau lượt thi Về đích của mỗi thí sinh. Nếu các thí sinh có cùng số điểm thì người có số thứ tự đứng nhỏ hơn sẽ là người dự thi trước.
    Thí sinh đang trả lời gói câu hỏi của mình phải đưa ra câu trả lời trong thời gian quy định. Nếu thí sinh trả lời đúng sẽ ghi được điểm của câu hỏi. Nếu không trả lời được câu hỏi thì các thí sinh còn lại có 5 giây để bấm chuông trả lời. Chỉ một người nhấn chuông nhanh nhất được quyền trả lời, trả lời đúng được cộng thêm ${stealGain} từ thí sinh đang thi. Trả lời sai ${lossClause(s.steal_wrong_percent)}.
    Thí sinh có quyền được đặt ngôi sao hy vọng 1 lần trước bất kỳ câu hỏi nào. Trả lời đúng được ${gainOf(s.star_correct_percent)}, trả lời sai ${lossClause(s.star_wrong_percent, ' đặt NSHV')}.`;
}

/** Legacy rule slide (title, text, legacy font size in vh) for a round; khoi_dong for unknown phases. */
export function ruleText(phase: string, scoring: ScoringRules = DEFAULT_SCORING): RuleText {
	switch (phase) {
		case 'tang_toc':
			return { name: 'TĂNG TỐC', size: 3.6, content: tangToc(scoring.tang_toc) };
		case 'vcnv':
			return { name: 'VƯỢT CHƯỚNG NGẠI VẬT', size: 3.28, content: vcnv(scoring.vcnv) };
		case 've_dich':
			return { name: 'VỀ ĐÍCH', size: 3.4, content: veDich(scoring.ve_dich) };
		default:
			return { name: 'KHỞI ĐỘNG', size: 4.3, content: khoiDong(scoring.khoi_dong) };
	}
}
