module github.com/kodylow/base58-website

go 1.19

require (
	github.com/akamensky/base58 v0.0.0-20210829145138-ce8bf8802e8f
	github.com/alexedwards/scs/v2 v2.5.0
	github.com/base58btc/mailer v0.0.0-20230510213939-04e8f45514b5
	github.com/btcsuite/btcutil v1.0.2
	github.com/gomarkdown/markdown v0.0.0-20250311123330-531bef5e742b
	github.com/google/go-cmp v0.6.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/schema v1.4.1
	github.com/joho/godotenv v1.5.1
	github.com/rs/cors v1.11.1
	github.com/sorcererxw/go-notion v0.2.4
	github.com/stripe/stripe-go/v74 v74.12.0
)

require (
	github.com/jmoiron/sqlx v1.3.5 // indirect
	github.com/json-iterator/go v1.1.10 // indirect
	github.com/mailgun/mailgun-go/v4 v4.8.2 // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v0.0.0-20180701023420-4b7aa43c6742 // indirect
	github.com/pkg/errors v0.8.1 // indirect
	github.com/sendgrid/rest v2.6.9+incompatible // indirect
	github.com/sendgrid/sendgrid-go v3.12.0+incompatible // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/sorcererxw/go-notion v0.2.4 => github.com/niftynei/go-notion v0.0.0-20250701021727-e8f91f2e9b6d
