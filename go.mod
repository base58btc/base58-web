module github.com/kodylow/base58-website

go 1.24.0

toolchain go1.24.4

require (
	github.com/akamensky/base58 v0.0.0-20210829145138-ce8bf8802e8f
	github.com/alexedwards/scs/v2 v2.5.0
	github.com/base58btc/mailer v0.0.0-20250703225241-27f47714b725
	github.com/btcsuite/btcutil v1.0.2
	github.com/gomarkdown/markdown v0.0.0-20250311123330-531bef5e742b
	github.com/google/go-cmp v0.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/schema v1.4.1
	github.com/jackc/pgx/v5 v5.7.6
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-sqlite3 v1.14.28
	github.com/rs/cors v1.11.1
	github.com/sorcererxw/go-notion v0.2.4
	github.com/stripe/stripe-go/v74 v74.12.0
)

require (
	github.com/go-chi/chi/v5 v5.2.2 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jmoiron/sqlx v1.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailgun/errors v0.4.0 // indirect
	github.com/mailgun/mailgun-go/v4 v4.23.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/sendgrid/rest v2.6.9+incompatible // indirect
	github.com/sendgrid/sendgrid-go v3.16.1+incompatible // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/sorcererxw/go-notion v0.2.4 => github.com/niftynei/go-notion v0.0.0-20250701021727-e8f91f2e9b6d
