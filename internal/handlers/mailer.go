package handlers

import (
	"fmt"

	"github.com/kodylow/base58-website/internal/types"
	"github.com/kodylow/base58-website/internal/config"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func SendWaitlistConfirmed(ctx *config.AppContext, email string, session *types.CourseSession) error {
	from := mail.NewEmail("Base58⛓🔓", "hello@base58.school")
	subject := fmt.Sprintf("[Base58] You're on the List: %s", session.CourseName)
	to := mail.NewEmail("🐲", email)

	plainTextContent := "Sit tight, you're on our list"
	htmlContent := "<strong>Sit tight, you're on our list!</strong>"
	message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)
	client := sendgrid.NewSendClient(ctx.Env.SendGrid.Key)
	response, err := client.Send(message)
	if err == nil {
		fmt.Println(response.StatusCode)
		fmt.Println(response.Body)
		fmt.Println(response.Headers)
	}

	return err
}
