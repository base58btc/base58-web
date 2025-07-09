package handlers

import (
	"strings" 

	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
	"github.com/kodylow/base58-website/internal/types"
)

func ScheduleMissives(ctx *config.AppContext, subscribers []*types.Subscriber, newsletter string) error {

	letters, err := getters.GetLetters(ctx.Notion, newsletter)
	if err != nil {
		return err
	}

	for _, letter := range letters {
		if !letter.Sendable() {
			continue
		}

		if letter.AtSubOnly() {
			continue
		}

		sendAt, err := letter.CalcSendAt()
		if err != nil {
			return err
		}

		for _, sub := range subscribers {
			if !sub.IsSubscribed(newsletter) {
				continue
			}

			_, err := emails.SendNewsletterMissive(ctx, sub.Email, letter, sendAt)
			if err != nil {
				/* FIXME: do something less hacky for collisions 
				(like returning a specific error code)
				*/
				if !strings.Contains(err.Error(), "scheduled.idem_key") {
					return err
				}
			}
		}
	}

	return nil
}

func NewSubscriberMissives(ctx *config.AppContext, email, newsletter string) error {

	letters, err := getters.GetLetters(ctx.Notion, newsletter)
	if err != nil {
		return err
	}

	for _, letter := range letters {
		if !letter.Sendable() {
			continue
		}

		sendAt, err := letter.CalcSendAt()
		if err != nil {
			return err
		}

		_, err = emails.SendNewsletterMissive(ctx, email, letter, sendAt)
		if err != nil {
			/* FIXME: do something less hacky for collisions */
			if !strings.Contains(err.Error(), "scheduled.idem_key") {
				return err
			}
		}
	}

	return nil
}
