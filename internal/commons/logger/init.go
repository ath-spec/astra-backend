package logger

func InitLogger(service Microservice) {
	l = &serialLogger{
		Microservice: service,
	}
}
