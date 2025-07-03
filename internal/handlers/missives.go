package handlers

import (
	"strings" 

	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
)

func ScheduleMissives(ctx *config.AppContext, email, newsletter string) error {

	letters, err := getters.GetLetters(ctx.Notion, newsletter)
	if err != nil {
		return err
	}

	for _, letter := range letters {
		if letter.IsExpired() {
			continue
		}

		_, err := emails.SendNewsletterMissive(ctx, email, letter)
		if err != nil {
			/* FIXME: do something less hacky for collisions */
			if !strings.Contains(err.Error(), "scheduled.idem_key") {
				return err
			}
		}
	}

	return nil
}
