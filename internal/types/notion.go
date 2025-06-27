package types

import (
	"github.com/sorcererxw/go-notion"
)

type (
	NotionConfig struct {
		Token          string
		CoursesDb      string `toml:"courses"`
		SessionsDb     string `toml:"upcoming_sessions"`
		SessionCartsDb string `toml:"session_carts"`
		ItemCartsDb    string `toml:"item_carts"`
		SignupsDb      string `toml:"signups"`
		WaitlistDb     string `toml:"class_waitlist"`
		NewsletterDb   string `toml:"newsletter"`
	}

	Notion struct {
		Config NotionConfig
		Client notion.API
	}
)

func (n *Notion) Setup() {
	client := notion.NewClient(notion.Settings{Token: n.Config.Token})
	n.Client = client
}
