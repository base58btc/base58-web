module github.com/kodylow/base58-website

go 1.19

require (
	github.com/BurntSushi/toml v1.2.1
	github.com/alexedwards/scs/v2 v2.5.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/schema v1.2.0
	github.com/joncalhoun/form v1.0.1
	github.com/sendgrid/sendgrid-go v3.12.0+incompatible
	github.com/sorcererxw/go-notion v0.2.4
	github.com/stripe/stripe-go/v74 v74.12.0
)

require (
	github.com/sendgrid/rest v2.6.9+incompatible // indirect
	github.com/stretchr/testify v1.8.1 // indirect
)

replace github.com/sorcererxw/go-notion v0.2.4 => github.com/niftynei/go-notion v0.0.0-20230323155332-a2c93bab119e
