package getters

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/kodylow/base58-website/internal/types"
)

func loadConfig() (*types.EnvConfig, bool) {
	var config types.EnvConfig
	var err error

	/* no toml file.. */
	if secrets := os.Getenv("SECRETS_FILE"); secrets != "" {
		fmt.Println("using secrets", secrets)
		err = godotenv.Load(secrets)
	} else if _, err := os.Stat("../../.env"); err == nil {
		err = godotenv.Load("../../.env")
		fmt.Printf("Loading from .env file\n")
	}
	if err != nil {
		log.Fatal(err)
	}

	config.Port = os.Getenv("PORT")
	config.Domain = os.Getenv("LOCAL_DOMAIN")
	config.External = os.Getenv("EXTERN_DOMAIN")
	config.Secret = os.Getenv("TOKEN_SEC")
	config.MailerSecret = os.Getenv("MAIL_SEC")
	config.MailDomain = os.Getenv("MAIL_DOMAIN")
	config.MailEndpoint = os.Getenv("MAIL_API")

	config.Commando = types.CommandoConfig{
		Host:   os.Getenv("LN_HOST"),
		NodeID: os.Getenv("LN_NODE_ID"),
		Rune:   os.Getenv("LN_RUNE"),
	}

	isProd := os.Getenv("IS_PROD") == "1"

	return &config, isProd
}

func TestNewInvoice(t *testing.T) {
	env, _ := loadConfig()
	cmdo := env.Commando
	description := "a test invoice"
	var amt uint64 = 1000
	msg, label, err := NewInvoice(&cmdo, description, amt)
	if err != nil {
		t.Fatalf(`Error in NewInvoice(cmdo, description, amt) = %q, %v`, msg, err)
	}
	fmt.Printf("Invoice = %v, label = %s\n", msg, label)

	msg, err = WaitInvoice(&cmdo, label)
	if err != nil {
		t.Fatalf(`Error in WaitInvoice(cmdo, label) = %q, %v`, msg, err)
	}
	fmt.Printf("WaitInvoice Found = %v\n", msg)
}
