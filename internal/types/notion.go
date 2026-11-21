package types

import (
	"github.com/sorcererxw/go-notion"
)

type (
	NotionConfig struct {
		Token      string
		CoursesDb  string `toml:"courses"`
		SessionsDb string `toml:"upcoming_sessions"`
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
