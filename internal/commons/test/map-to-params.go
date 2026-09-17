package test

import "fmt"

func ConvertMapToQueryParams(baseUrl string, params map[string]interface{}) string {
	baseUrl += "?"
	for k, v := range params {
		baseUrl += fmt.Sprintf("%s=%v&", k, v)
	}
	return baseUrl
}
