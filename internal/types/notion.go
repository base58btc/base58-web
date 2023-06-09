package types

import (
	"github.com/sorcererxw/go-notion"
)

type (
	NotionConfig struct {
		Token      string
		CoursesDb  string `toml:"available_courses"`
		SessionsDb string `toml:"upcoming_sessions"`
		CartsDb    string `toml:"carts"`
		SignupsDb  string `toml:"signups"`
		WaitlistDb string `toml:"class_waitlist"`
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
