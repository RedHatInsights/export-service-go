package exports

import (
	"fmt"
	"net/url"
	"time"

	"github.com/redhatinsights/export-service-go/models"
)

const (
	formatDate     = "2006-01-02"
	formatDateLen  = 10
	formatDateTime = time.RFC3339
)

func initQuery(q url.Values) (result models.QueryParams, err error) {
	// all params are optional
	result.Name = q.Get("name")
	result.Application = q.Get("application")
	result.Resource = q.Get("resource")
	result.Status = q.Get("status")

	created := q.Get("created")
	expires := q.Get("expires")

	// created and expires should be date only, not date-time strings
	if created != "" {
		result.Created, err = parseDate(created)

		if err != nil {
			return models.QueryParams{}, fmt.Errorf("'%s' is not a valid date in ISO 8601", created)
		}
	}

	if expires != "" {
		result.Expires, err = parseDate(expires)
		if err != nil {
			return models.QueryParams{}, fmt.Errorf("'%s' is not a valid date in ISO 8601", expires)
		}
	}

	return
}

func parseDate(str string) (result time.Time, err error) {
	format := formatDateTime
	// if the strings length is as long as formatDate then
	if len(str) == formatDateLen {
		format = formatDate
	}

	result, err = time.Parse(format, str)

	return
}
