package game

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// khoiDongSourceLabel accepts only canonical question IDs and returns the exact
// label used in score-draft reasons.
func khoiDongSourceLabel(id string) (string, bool) {
	parts := strings.Split(id, ".")
	indexText := ""
	owner := 0
	if len(parts) == 2 && parts[0] == "kd_chung" {
		indexText = parts[1]
	} else if len(parts) == 3 && parts[0] == "kd_rieng" {
		n, err := strconv.Atoi(parts[1])
		if err != nil || n < 0 || n > 3 || strconv.Itoa(n) != parts[1] {
			return "", false
		}
		owner, indexText = n+1, parts[2]
	} else {
		return "", false
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 || index == int(^uint(0)>>1) || strconv.Itoa(index) != indexText {
		return "", false
	}
	if owner == 0 {
		return fmt.Sprintf("Khởi động chung · Câu %d", index+1), true
	}
	return fmt.Sprintf("Khởi động · TS%d câu %d", owner, index+1), true
}

func khoiDongReasonID(label string) string {
	var owner, question int
	var id string
	if n, _ := fmt.Sscanf(label, "Khởi động chung · Câu %d", &question); n == 1 && question > 0 {
		id = fmt.Sprintf("kd_chung.%d", question-1)
	} else if n, _ := fmt.Sscanf(label, "Khởi động · TS%d câu %d", &owner, &question); n == 2 && owner >= 1 && owner <= 4 && question > 0 {
		id = fmt.Sprintf("kd_rieng.%d.%d", owner-1, question-1)
	}
	canonical, ok := khoiDongSourceLabel(id)
	if ok && canonical == label {
		return id
	}
	return ""
}

// Called under s.mu: neither a quick judgment nor another score save can
// interleave between the durable history check and the subsequent mutation.
func (s *Service) validateScoreSourcesLocked(ctx context.Context, reason string, sources []ScoreAdjustmentSource) error {
	byQuestion := make(map[string]ScoreAdjustmentSource, len(sources))
	for _, source := range sources {
		if _, ok := khoiDongSourceLabel(source.QuestionID); !ok || source.Version < 0 || source.Version > s.state.Version ||
			(source.Kind != "verdict" && source.Kind != "correction" && (source.Kind != "" || source.kindProvided)) {
			return validation("sources", "Nguồn bản nháp Khởi động không hợp lệ; hãy bỏ và tạo lại bản nháp.")
		}
		if previous, exists := byQuestion[source.QuestionID]; exists {
			if previous.Version < source.Version {
				source.Version = previous.Version
			}
			if previous.Kind != "correction" {
				source.Kind = "verdict"
			}
		}
		byQuestion[source.QuestionID] = source
	}
	reason = strings.TrimSpace(reason)
	legacyUndo := strings.HasPrefix(reason, "Hoàn tác · ")
	if legacyUndo {
		reason = strings.TrimPrefix(reason, "Hoàn tác · ")
		// Legacy undo is identified only by its exact question labels, never
		// by possibly stale or unrelated source metadata.
		byQuestion = make(map[string]ScoreAdjustmentSource)
	}
	for _, item := range strings.Split(reason, ",") {
		id := khoiDongReasonID(strings.TrimSpace(item))
		if id != "" {
			if _, exists := byQuestion[id]; !exists {
				byQuestion[id] = ScoreAdjustmentSource{QuestionID: id}
			}
		}
	}
	if len(byQuestion) == 0 {
		return nil
	}
	events, err := s.repository.ListEvents(ctx, s.state.MatchID)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Type != "KhoiDong.Judged" {
			continue
		}
		var payload struct {
			QuestionID string `json:"question_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		source, exists := byQuestion[payload.QuestionID]
		if exists && (legacyUndo || source.Kind != "correction" || source.Version == 0 || event.State == nil || event.State.Version <= 0 || event.State.Version > source.Version) {
			if legacyUndo {
				return validation("reason", "Câu Khởi động đã được chấm nhanh; không thể hoàn tác bản lưu cũ. Hãy dùng +/- để điều chỉnh điểm thủ công.")
			}
			return validation("sources", "Câu Khởi động đã được chấm nhanh; hãy bỏ bản nháp chấm điểm cũ và dùng +/- thủ công để điều chỉnh điểm.")
		}
	}
	return nil
}
