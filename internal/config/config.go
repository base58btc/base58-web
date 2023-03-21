package config

import (
	"html/template"
	"log"

	"github.com/alexedwards/scs/v2"
	"github.com/kodylow/base58-website/internal/types"
)

// AppConfig holds the application configuration settings
type AppConfig struct {
	InProduction  bool
	InfoLog       *log.Logger
	ErrorLog      *log.Logger
	Session       *scs.SessionManager
	TemplateCache map[string]*template.Template

	Context types.AppContext
}
