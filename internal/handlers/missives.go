package handlers

import (
	"strings"
	"time"

	"github.com/kodylow/base58-website/external/getters"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/emails"
	"github.com/kodylow/base58-website/internal/types"
)

func ScheduleMissives(ctx *config.AppContext, subscribers []*types.Subscriber, newsletter string) error {

	subonly, sendable, skipped := 0, 0, 0
	letters, err := getters.GetLetters(ctx.Notion, newsletter)
	if err != nil {
		return err
	}

	for _, letter := range letters {
		if !letter.Sendable() {
			skipped += 1
			continue
		}

		if letter.AtSubOnly() {
			subonly += 1
			continue
		}

		sendAt, err := letter.CalcSendAt()
		if err != nil {
			return err
		}

		sendable += 1
		subssent := 0
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
			subssent += 1
		}
		ctx.Infos.Printf("Sent %d emails (%s)", subssent, letter.Title)

		/* Update SentAt field! */
		if letter.SetSentAt() {
			now := time.Now()
			err = getters.MarkLetterSent(ctx.Notion, letter, now)
			if err != nil {
				return err
			}
		}
	}

	ctx.Infos.Printf("Attempted to send %d; skipped %d 'subonly' %d sent %d", len(letters), skipped, subonly, sendable)

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
