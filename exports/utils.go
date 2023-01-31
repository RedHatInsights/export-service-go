package exports

import (
	"fmt"
	"net/url"
	"time"

	"github.com/redhatinsights/export-service-go/models"
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
	format := "2006-01-02"
	if created != "" {
		result.Created, err = time.Parse(time.RFC822, created)
		if err != nil {
			return models.QueryParams{}, fmt.Errorf("'%s' is not a valid date-time", created)
		}
	}

	if expires != "" {
		result.Expires, err = time.Parse(format, expires)
		if err != nil {
			return models.QueryParams{}, fmt.Errorf("'%s' is not a valid date-time", expires)
		}
	}

	return
}
