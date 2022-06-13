package exports

import (
	"fmt"
	"net/url"
	"strings"
)

// The readString() helper returns a string value from the query string, or the provided
// default value if no matching key could be found.
func (e *Export) readString(qs url.Values, key string, defaultValue string) string {
	// Extract the value for a given key from the query string. If no key exists this
	// will return the empty string "".
	s := qs.Get(key)
	// If no key exists (or the value is empty) then return the default value.
	if s == "" {
		return defaultValue
	}
	// Otherwise return the string.
	return s
}

// The readCSV() helper reads a string value from the query string and then splits it
// into a slice on the comma character. If no matching key could be found, it returns
// the provided default value.
func (e *Export) readCSV(qs url.Values, key string, defaultValue []string) []string {
	// Extract the value from the query string.
	csv := qs.Get(key)
	// If no key exists (or the value is empty) then return the default value.
	if csv == "" {
		return defaultValue
	}
	// Otherwise parse the value into a []string slice and return it.
	return strings.Split(csv, ",")
}

func (e *Export) convertSortParams(in []string) []string {
	valids := map[string]struct{}{"name": {}, "-name": {}, "created": {}, "-created": {}, "expires": {}, "-expires": {}, "application": {}, "-application": {}, "resource": {}, "-resource": {}}
	result := []string{}
	for _, s := range in {
		if _, ok := valids[s]; !ok {
			continue
		}
		if strings.HasPrefix(s, "-") {
			s = fmt.Sprintf("%s desc", strings.TrimPrefix(s, "-"))
			result = append(result, s)
		} else {
			s = fmt.Sprintf("%s asc", s)
			result = append(result, s)
		}
	}
	return result
}

// func buildFilterQuery(q url.Values) (map[string]interface{}, error) {
// 	result := map[string]interface{}{}

// 	valid := map[string]string{"name": "name", "create": "created_at", "expires": "expires", "application": "sources @> ?", "resource": "sources @> ?"}

// 	/*
// 		name
// 		created
// 		expires
// 		application
// 		resource
// 	*/

// 	for k, v := range q {
// 		field, ok := valid[k]
// 		if !ok {
// 			continue
// 		}
// 		if len(v) > 1 {
// 			return nil, fmt.Errorf("query param `%s` has too many search values", k)
// 		}
// 		result[field] = v[0]
// 	}

// 	return result, nil
// }
