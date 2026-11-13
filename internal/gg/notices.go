package gg

import (
	"strings"
	"time"
)

const successNoticeDuration = 5 * time.Second

func (p *Project) setNotice(message string, success bool) {
	_ = p.locked(func(s *State) error {
		s.Notice, s.NoticeKind, s.NoticeUntil = message, "error", time.Time{}
		if success {
			s.NoticeKind, s.NoticeUntil = "success", time.Now().UTC().Add(successNoticeDuration)
		}
		return nil
	})
}

func (s *State) visibleNotice(now time.Time) (string, bool) {
	if s == nil {
		return "", false
	}
	success := s.NoticeKind == "success"
	if s.NoticeKind == "" && (strings.HasPrefix(s.Notice, "Completed: ") || strings.HasPrefix(s.Notice, "Выполнено: ")) {
		return "", false // Old success has no trustworthy completion time.
	}
	if success && (s.NoticeUntil.IsZero() || !now.Before(s.NoticeUntil)) {
		return "", false
	}
	return s.Notice, success
}
