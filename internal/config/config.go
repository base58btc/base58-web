package config

import (
	"fmt"
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
	Session       *scs.SessionManager
	TemplateCache *template.Template
	EmailCache    map[string]*template.Template
	DocCache      map[string][]byte
	Notion        *types.Notion

	Env *types.EnvConfig
}

func (ctx *AppContext) SitePath() string {
	var prefix, port string
	if ctx.IsProd {
		prefix = "https://"
		port = ""
	} else {
		prefix = "http://"
		port = ":" + ctx.Env.Port
	}

	return fmt.Sprintf("%s%s%s", prefix, ctx.Env.Domain, port)
}

func (ctx *AppContext) CallbackPath() string {
	if ctx.IsProd {
		return ctx.SitePath()
	}

	return ctx.Env.External
}
