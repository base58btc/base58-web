package config

import (
	"html/template"
	"log"

	"github.com/alexedwards/scs/v2"
	"github.com/kodylow/base58-website/internal/types"
)

/* AppContext holds the application settings + globals */
type AppContext struct {
	IsProd        bool
	Infos         *log.Logger
	Err           *log.Logger
	Redraw        bool
	Session       *scs.SessionManager
	TemplateCache map[string]*template.Template
	Notion *types.Notion

	Env *types.EnvConfig
}

func (ctx *AppContext) ReloadCache() bool {
	return !ctx.IsProd && ctx.Redraw
}
