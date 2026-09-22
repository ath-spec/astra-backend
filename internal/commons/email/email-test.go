package email

type emailTest struct {
}

func NewEmailSenderTest(from, password string) EmailSender {
	return &emailTest{}
}

func (e *emailTest) SendMail(to []string, subject string, attrs map[string]interface{}) error {
	return nil
}
