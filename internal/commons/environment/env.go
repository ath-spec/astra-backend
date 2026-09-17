package environment

import (
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func LoadEnv(config any) error {
	// Try loading .env and .env.local files from multiple potential locations.
	// Standard convention: .env.local overwrites .env
	
	dirs := []string{"./", "../", "../../"}
	
	// 1. Load .env files first (no overwrite)
	for _, dir := range dirs {
		_ = godotenv.Load(dir + ".env")
	}
	
	// 2. Load .env.local files second (overload/overwrite)
	for _, dir := range dirs {
		_ = godotenv.Overload(dir + ".env.local")
	}

	value := reflect.ValueOf(config)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Struct {
		return errors.New("config must be a pointer to a struct")
	}

	value = value.Elem()
	valueType := value.Type()

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldType := valueType.Field(i)
		envTag := fieldType.Tag.Get("env")

		if envTag == "" {
			continue
		}

		// Check if field is optional
		isOptional := strings.Contains(envTag, ",optional")
		envName := strings.Split(envTag, ",")[0]

		envValue, ok := os.LookupEnv(envName)
		if !ok {
			if isOptional {
				continue // Skip optional fields that are not set
			}
			return errors.New("missing environment variable: " + envName)
		}

		if !field.CanSet() {
			return errors.New("cannot set field value: " + fieldType.Name)
		}

		switch field.Type().Kind() {
		case reflect.String:
			field.SetString(envValue)

		case reflect.Slice:
			if fieldType.Type.Elem().Kind() != reflect.String {
				return errors.New("slice elements must be of type strings")
			}

			envValues := strings.Split(envValue, ",")
			field.Set(reflect.ValueOf(envValues))

		case reflect.Bool:
			val, err := strconv.ParseBool(strings.TrimSpace(envValue))
			if err != nil {
				return errors.New("invalid boolean value for " + envName)
			}
			field.SetBool(val)

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
			val, err := strconv.ParseInt(strings.TrimSpace(envValue), 10, 64)
			if err != nil {
				return errors.New("invalid integer value for " + envName)
			}
			field.SetInt(val)

		case reflect.Int64:
			if field.Type().String() == "time.Duration" {
				dur, err := time.ParseDuration(strings.TrimSpace(envValue))
				if err != nil {
					return err
				}
				field.Set(reflect.ValueOf(dur))
			} else {
				val, err := strconv.ParseInt(strings.TrimSpace(envValue), 10, 64)
				if err != nil {
					return errors.New("invalid integer value for " + envName)
				}
				field.SetInt(val)
			}

		case reflect.Float32, reflect.Float64:
			val, err := strconv.ParseFloat(strings.TrimSpace(envValue), 64)
			if err != nil {
				return errors.New("invalid float value for " + envName)
			}
			field.SetFloat(val)
		}
	}

	return nil
}
