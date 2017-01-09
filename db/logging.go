package db

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"regexp"
	"time"
	"unicode"

	"github.com/Sirupsen/logrus"
)

type logrusAdapter struct {
	logger logrus.FieldLogger
}

var (
	sqlRegexp    = regexp.MustCompile(`(\$\d+)|\?`)
	newlineregex = regexp.MustCompile(`(?m)\n^\s+`)
)

func (l logrusAdapter) Print(t time.Duration, query string, args []interface{}) {
	messages := []interface{}{}

	query = newlineregex.ReplaceAllString(query, " ")

	// duration
	messages = append(messages, fmt.Sprintf("db: \033[36;1m[%.2fms]\033[0m ", float64(t.Nanoseconds()/1e4)/100.0))
	// sql
	var sql string
	var formattedValues []string

	for _, value := range args {
		indirectValue := reflect.Indirect(reflect.ValueOf(value))
		if indirectValue.IsValid() {
			value = indirectValue.Interface()
			if t, ok := value.(time.Time); ok {
				formattedValues = append(formattedValues, fmt.Sprintf("'%v'", t.Format(time.RFC3339)))
			} else if b, ok := value.([]byte); ok {
				if str := string(b); isPrintable(str) {
					formattedValues = append(formattedValues, fmt.Sprintf("'%v'", str))
				} else {
					formattedValues = append(formattedValues, "'<binary>'")
				}
			} else if r, ok := value.(driver.Valuer); ok {
				if value, err := r.Value(); err == nil && value != nil {
					formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
				} else {
					formattedValues = append(formattedValues, "NULL")
				}
			} else {
				formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
			}
		} else {
			formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
		}
	}

	var formattedValuesLength = len(formattedValues)
	for index, value := range sqlRegexp.Split(query, -1) {
		sql += value
		if index < formattedValuesLength {
			sql += formattedValues[index]
		}
	}

	messages = append(messages, sql)
	l.logger.Debugln(messages...)
}
func isPrintable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
