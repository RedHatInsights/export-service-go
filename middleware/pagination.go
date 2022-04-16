package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	"github.com/redhatinsights/export-service-go/errors"
)

type paginationKey int

const (
	PaginateKey   paginationKey = iota
	defaultLimit  int           = 100
	defaultOffset int           = 0
)

type Paginate struct {
	Limit  int
	Offset int
}

type PaginatedResponse struct {
	Meta  Meta        `json:"meta"`
	Links Links       `json:"links"`
	Data  interface{} `json:"data"`
}

func lenSlice(data interface{}) int {
	switch reflect.TypeOf(data).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(data)
		return s.Len()
	}
	return -1
}

func indexSlice(data interface{}, start, stop int) interface{} {
	switch reflect.TypeOf(data).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(data)
		len := s.Len()
		if len == 0 {
			return s
		}
		if start > len {
			return []interface{}{}
		}
		if stop > len {
			stop = len
		}
		return s.Slice(start, stop).Interface()
	}
	return -1
}

func GetPaginatedResponse(url *url.URL, p Paginate, data interface{}) (*PaginatedResponse, error) {
	// use lenSlice as error checker. lenSlice returns -1 if the data is not a slice.
	// Paginated data must be a slice
	if lenSlice(data) == -1 {
		return nil, fmt.Errorf("invalid data set: must be a slice")
	}
	return &PaginatedResponse{
		Meta:  getMeta(data),
		Links: getLinks(url, p, data),
		Data:  indexSlice(data, p.Offset, p.Offset+p.Limit),
	}, nil
}

type Meta struct {
	Count int `json:"count"`
}

func getMeta(data interface{}) Meta {
	return Meta{Count: lenSlice(data)}
}

type Links struct {
	First    string  `json:"first"`
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	Last     *string `json:"last"`
}

func getFirstLink(url *url.URL) string {
	firstURL := url
	q := firstURL.Query()
	q.Del("offset")

	firstURL.RawQuery = q.Encode()
	return firstURL.String()
}

func getNextLink(url *url.URL, count, limit, offset int) *string {
	if offset+limit > count {
		return nil
	}
	nextURL := url
	q := nextURL.Query()
	q.Set("offset", fmt.Sprintf("%d", limit+offset))

	nextURL.RawQuery = q.Encode()
	next := nextURL.String()

	return &next
}

func getPreviousLink(url *url.URL, count, limit, offset int) *string {
	if offset <= 0 {
		return nil
	}
	previousURL := url
	q := previousURL.Query()

	if offset-limit <= 0 {
		q.Del("offset")
	} else {
		q.Set("offset", fmt.Sprintf("%d", offset-limit))
	}

	previousURL.RawQuery = q.Encode()
	previous := previousURL.String()

	return &previous
}

func getLastLink(url *url.URL, count, limit, offset int) *string {
	if count-limit <= 0 {
		return nil
	}
	lastURL := url
	q := lastURL.Query()
	q.Set("offset", fmt.Sprintf("%d", count-limit))

	lastURL.RawQuery = q.Encode()
	last := lastURL.String()

	return &last
}

func getLinks(url *url.URL, p Paginate, data interface{}) Links {
	result := Links{First: getFirstLink(url)}
	count := lenSlice(data)
	if count <= p.Limit && p.Offset == 0 {
		return result
	}
	result.Next = getNextLink(url, count, p.Limit, p.Offset)
	result.Previous = getPreviousLink(url, count, p.Limit, p.Offset)
	result.Last = getLastLink(url, count, p.Limit, p.Offset)

	return result
}

func PaginationCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pagination := Paginate{
			Limit:  defaultLimit,
			Offset: defaultOffset,
		}
		limit := r.URL.Query().Get("limit")
		if limit != "" {
			lim, err := strconv.Atoi(limit)
			if err != nil {
				errors.BadRequestError(w, fmt.Errorf("invalid limit: %v", err))
				return
			}
			pagination.Limit = lim
		}

		offset := r.URL.Query().Get("offset")
		if offset != "" {
			off, err := strconv.Atoi(offset)
			if err != nil {
				errors.BadRequestError(w, fmt.Errorf("invalid offset: %v", err))
				return
			}
			pagination.Offset = off
		}

		ctx := context.WithValue(r.Context(), PaginateKey, pagination)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetPagination(ctx context.Context) Paginate {
	return ctx.Value(PaginateKey).(Paginate)
}
