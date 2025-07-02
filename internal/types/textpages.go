package types

type (
	TextPage struct {
		Tag    string
		Title  string
	}
)

var TextPages []TextPage = []TextPage{
	TextPage{
		Tag: "privacy",
		Title: "Privacy Policy",
	},
	TextPage{
		Tag: "terms",
		Title: "Terms of Service",
	},
}
