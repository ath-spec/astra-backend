package email

import (
	"bytes"
	"net/smtp"
	"text/template"
)

var auth smtp.Auth
var mime, addr string
var templatePath string

func setupAuth(email, password string) {
	auth = smtp.PlainAuth("", email, password, "smtp.gmail.com")
	mime = "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	addr = "smtp.gmail.com:587"
	templatePath = "./email-template.html"
}

type EmailSender interface {
	SendMail(to []string, subject string, attrs map[string]interface{}) error
}

type email struct {
	from         string
	to           []string
	subject      string
	templateData map[string]interface{}
	body         string
}

func NewEmailSender(from, password string) EmailSender {
	if auth == nil {
		setupAuth(from, password)
	}

	return &email{
		from: from,
	}
}

func (e *email) SendMail(to []string, subject string, attrs map[string]interface{}) error {
	e.to = to
	e.subject = subject
	e.templateData = attrs

	if err := e.parseTemplate(e.templateData); err != nil {
		return err
	}

	sub := "Subject: " + e.subject + "!\n"
	msg := []byte(sub + mime + "\n" + e.body)

	return smtp.SendMail(addr, auth, e.from, e.to, msg)
}

func (e *email) parseTemplate(data any) error {
	t, err := template.ParseFiles(templatePath)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	if err := t.Execute(buf, data); err != nil {
		return err
	}

	e.body = buf.String()
	return nil
}
