package util

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Converts a string array to an any array.
func ConvertStringArrayToAnyArray(input []string) []any {
	args := make([]any, len(input))
	for i, v := range input {
		args[i] = v
	}
	return args
}

// Repeats an input for a given amount.
func RepeatInput[T any | string | int](input T, amount int) []T {
	arr := make([]T, amount)
	for i := 0; i < amount; i++ {
		arr[i] = input
	}
	return arr
}

// Converts a struct to an any array.
func ConvertStructToAnyArray(entry interface{}) ([]any, error) {
	val := reflect.ValueOf(entry)
	if val.Kind() != reflect.Struct {
		return nil, errors.New("input is not of type struct")
	}

	numFields := val.NumField()
	slice := make([]any, numFields)

	for i := 0; i < numFields; i++ {
		field := val.Field(i).Interface()
		slice[i] = field
	}

	return slice, nil
}

// Converts a struct's fields pointers to an any array.
func ConvertStructPointerFieldsToPointerAnyArray(entry any) ([]any, error) {
	ptrValue := reflect.ValueOf(entry)

	if ptrValue.Kind() != reflect.Ptr || ptrValue.Elem().Kind() != reflect.Struct {
		return nil, errors.New("input is not of type pointer of struct")
	}

	structType := ptrValue.Elem().Type()
	structValue := ptrValue.Elem()

	fieldPointers := make([]any, structType.NumField())

	for i := 0; i < structType.NumField(); i++ {
		fieldValue := structValue.Field(i).Addr().Interface()
		fieldPointers[i] = fieldValue
	}

	return fieldPointers, nil
}

// ConvertMapToInterface converts a map to an interface.
func ConvertMapToInterface(input map[string]interface{}, outStruct interface{}) error {
	val := reflect.ValueOf(outStruct).Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		fieldName := typ.Field(i).Tag.Get("cass")
		if fieldName == "" {
			continue // skip fields without the cass tag
		}

		mapValue, found := input[fieldName]
		if !found {
			continue // Skip fields not present in the map
		}

		field := val.Field(i)
		value := reflect.ValueOf(mapValue)

		if value.Type().ConvertibleTo(field.Type()) {
			field.Set(value.Convert(field.Type()))
		} else {
			return fmt.Errorf("type mismatch for field: %s", fieldName)
		}
	}

	return nil
}

func ConvertStringPointerToPgTypeString(input *string) pgtype.Text {
	if input == nil {
		return pgtype.Text{String: "", Valid: false}
	}
	return pgtype.Text{String: *input, Valid: true}
}

func ConvertFloat32PointerToPgTypeFloat8(input *float32) pgtype.Float8 {
	if input == nil {
		return pgtype.Float8{Float64: 0, Valid: false}
	}
	return pgtype.Float8{Float64: float64(*input), Valid: true}
}

func ConvertIntPointerToPgTypeTimestamptz(input *int) pgtype.Timestamptz {
	if input == nil {
		return pgtype.Timestamptz{Time: time.Now(), Valid: false}
	}
	return pgtype.Timestamptz{Time: time.Unix(int64(*input), 0), Valid: true}
}

var floatType = reflect.TypeOf(float64(0))

func ConvertInterfaceToFloat64(unk interface{}) (float64, error) {
	v := reflect.ValueOf(unk)
	v = reflect.Indirect(v)
	if !v.Type().ConvertibleTo(floatType) {
		return math.NaN(), fmt.Errorf("cannot convert %v to float64", v.Type())
	}
	fv := v.Convert(floatType)
	return fv.Float(), nil
}

func ConvertStringToFloat(input string) (float64, error) {
	f, err := strconv.ParseFloat(input, 64)
	if err != nil {
		fmt.Println("Error:", err)
		return 0, err
	}
	return f, nil
}

// PgNumericToFloat64 converts pgtype.Numeric to float64
func PgNumericToFloat64(numeric pgtype.Numeric) float64 {
	if !numeric.Valid {
		return 0.0
	}

	// Convert pgtype.Numeric to string first, then to float64
	str := numeric.Int.String()
	if numeric.Exp < 0 {
		// Handle decimal places
		exp := int(-numeric.Exp)
		if len(str) > exp {
			str = str[:len(str)-exp] + "." + str[len(str)-exp:]
		} else {
			str = "0." + fmt.Sprintf("%0*s", exp, str)
		}
	}

	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0.0
	}
	return f
}

// ConvertPgTextToStringPtr converts pgtype.Text to *string
func ConvertPgTextToStringPtr(text pgtype.Text) *string {
	if !text.Valid {
		return nil
	}
	return &text.String
}

// ConvertPgTimeToStringPtr converts pgtype.Time to *string
func ConvertPgTimeToStringPtr(time pgtype.Time) *string {
	if !time.Valid {
		return nil
	}
	// Convert microseconds to time format
	hours := time.Microseconds / 3600000000
	minutes := (time.Microseconds % 3600000000) / 60000000
	seconds := (time.Microseconds % 60000000) / 1000000
	microseconds := time.Microseconds % 1000000

	timeStr := fmt.Sprintf("%02d:%02d:%02d.%06d", hours, minutes, seconds, microseconds)
	return &timeStr
}

// ConvertUUIDToStringPtr converts uuid.UUID to *string
func ConvertUUIDToStringPtr(uuid pgtype.UUID) *string {
	if !uuid.Valid {
		return nil
	}
	uuidStr := fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid.Bytes[0:4],
		uuid.Bytes[4:6],
		uuid.Bytes[6:8],
		uuid.Bytes[8:10],
		uuid.Bytes[10:16])
	return &uuidStr
}

// ToPgText converts a string to pgtype.Text
func ToPgText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// ToPgDate converts a RFC3339 date string to pgtype.Date
func ToPgDate(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}
