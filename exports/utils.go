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
	formatDateTime = "2006-01-02T15:04:05Z" // ISO 8601
)

func initQuery(q url.Values) (result models.QueryParams, err error) {
	// all params are optional
	result.Name = q.Get("name")
	result.Application = q.Get("application")
	result.Resource = q.Get("resource")
	result.Status = q.Get("status")

	created := q.Get("created_at")
	expires := q.Get("expires_at")

	// created and expires can be in either format above
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

	return result, err
}

func parseDate(str string) (result time.Time, err error) {
	format := formatDateTime

	if len(str) == formatDateLen {
		format = formatDate
	}

	result, err = time.Parse(format, str)

	return result, err
}
