package sms

type senderTest struct{}

func NewSmsSenderTest() Sender {
	return &senderTest{}
}

func (s *senderTest) SendPointSms(authKey, flowID, mobile, senderID string, variables map[string]string) error {
	return nil
}

func (s *senderTest) Send2FactorSms(authKey, flowID, mobile string, otp int) error {
	return nil
}
