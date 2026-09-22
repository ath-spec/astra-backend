package connectors

// Infinite iterator that returns the Clickhouse session
// func CreateClickhouseConn(host []string, username, password, database string) clickhouse.Client {
// 	count := 0
// 	config := clickhouse.Config{
// 		Host:     host,
// 		Username: username,
// 		Password: password,
// 		Database: database,
// 	}
// 	for {
// 		conn, err := clickhouse.CreateClickhouseConn(config)
// 		if err != nil {
// 			count++
// 		} else {
// 			logger.Info("clickhouse connection created!")
// 			return conn
// 		}
// 		if count == 5 {
// 			logger.Error("unable to connect to clickhouse: %s", err)
// 			logger.Info("retrying in 5 seconds...")
// 			count = 0
// 			time.Sleep(time.Second * 5)
// 		}
// 	}
// }
