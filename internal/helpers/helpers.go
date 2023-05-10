package helpers

import (
	"time"

	"github.com/kodylow/base58-website/internal/types"
)

func FilterSessions(sessions []*types.CourseSession, from time.Time) []*types.CourseSession {
	filtered := make([]*types.CourseSession, 0)

	for _, sesh := range sessions {
		seshStart := sesh.Dates()[0]
		if !seshStart.Before(from.Add(time.Minute * 60)) {
			filtered = append(filtered, sesh)
		}
	}

	return filtered
}
