package cmd

import (
	"time"

	"github.com/dvhthomas/gh-velocity/internal/dateutil"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// parseDateWindow parses --since/--until flags into a time window.
// If untilStr is empty, now is used as the end of the window.
func parseDateWindow(sinceStr, untilStr string, now time.Time) (since, until time.Time, err error) {
	since, err = dateutil.Parse(sinceStr, now)
	if err != nil {
		return time.Time{}, time.Time{}, &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
	}

	until = now
	if untilStr != "" {
		until, err = dateutil.Parse(untilStr, now)
		if err != nil {
			return time.Time{}, time.Time{}, &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
		}
	}

	if err := dateutil.ValidateWindow(since, until, now); err != nil {
		return time.Time{}, time.Time{}, &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
	}

	return since, until, nil
}
