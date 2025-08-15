// filepath: /home/kartik7120/booking_auth_service/cmd/helper/mail.go
package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/go-playground/validator/v10"
)

type SendMailStruct struct {
	To       string
	Name     string
	Text     string
	Html     string
	Category string
	Subject  string
}

var validate *validator.Validate

// If this mail implementation is not working for us then we can also host mailhog on our local machine

func SendMail(s SendMailStruct) error {

	validate = validator.New()

	// validating if the email is valid
	log.Info("Starting to send email")

	err := validate.Var(s.To, "email")

	if err != nil {
		log.Error("Invalid email", err)
		return err
	}

	if len(s.Text) == 0 && len(s.Html) == 0 {
		log.Error("text or html is required")
		return fmt.Errorf("text or html is required")
	}

	// check if text or html does not contain any malicious code

	if len(s.Text) > 0 {
		err = validate.Var(s.Text, "printascii")

		if err != nil {
			log.Error("Invalid text", err)
			return err
		}
	}

	if len(s.Html) > 0 {

		err = validate.Var(s.Html, "printascii")

		if err != nil {
			log.Error("Invalid html")
			return err
		}

		err = validate.Var(s.Html, "html")

		if err != nil {
			log.Error("Invalid html", err)
			return err
		}

	}

	if len(s.Subject) > 0 {
		err = validate.Var(s.Subject, "printascii")

		if err != nil {
			log.Error("Invalid subject")
			return err
		}
	}

	url := "https://send.api.mailtrap.io/api/send"
	method := "POST"

	payloadString := ""

	if len(s.Html) > 0 {
		payloadString = fmt.Sprintf(`{
            "from": {
                "email": "hello@demomailtrap.co",
                "name": %s
            },
            "to": [
                {
                    "email": %s
                }
            ],
            "subject": %s,
            "html": %s,
            "category": %s
        }`, jsonEscape(s.Name), jsonEscape(s.To), jsonEscape(s.Subject), jsonEscape(s.Html), jsonEscape(s.Category))
	} else {
		payloadString = fmt.Sprintf(`{
                "from": {
                    "email": "hello@demomailtrap.co",
                    "name": %s
                },
                "to": [
                    {
                        "email": %s
                    }
                ],
                "subject": %s,
                "text": %s,
                "category": %s
            }`, jsonEscape(s.Name), jsonEscape(s.To), jsonEscape(s.Subject), jsonEscape(s.Html), jsonEscape(s.Category))
	}

	log.Info(payloadString)

	payload := strings.NewReader(payloadString)

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		log.Error("Error creating request")
		return err
	}

	tokenString := fmt.Sprintf("Bearer %s", os.Getenv("MAILTRAP_API_KEY"))

	req.Header.Add("Authorization", tokenString)
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		log.Error("Error sending email")
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)

	if err != nil {
		fmt.Println(err)
		log.Error("Error reading response")
		return err
	}

	log.Info(string(body))

	// check if body is valid to unmarshal

	if len(body) == 0 {
		log.Error("Empty response", err)
		return errors.New("empty response")
	}

	if len(body) > 0 {
		if body[0] != byte('{') {
			log.Error("Invalid response", err)
			return errors.New("invalid response")
		}
	}

	// convert body to json

	jsonBody := make(map[string]any)

	err = json.Unmarshal(body, &jsonBody)

	if err != nil {
		log.Error("Error unmarshalling response", err)
		return err
	}

	if jsonBody["success"] == false {
		log.Error("Error sending email", jsonBody["errors"])
		return errors.New("error sending email")
	}
	// check if the email was sent successfully

	log.Info("Email sent successfully")
	return nil
}

func jsonEscape(value string) string {
	escapedValue, _ := json.Marshal(value)
	return string(escapedValue)
}

func SendOTPMailTemplate(email, otp string) SendMailStruct {
	return SendMailStruct{
		To:       email,
		Name:     "User",
		Text:     fmt.Sprintf("Your OTP is %s", otp),
		Html:     fmt.Sprintf("<p>Your OTP is <strong>%s</strong></p>", otp),
		Category: "otp",
		Subject:  "Your OTP Code",
	}
}
