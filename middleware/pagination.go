/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
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

// Pagination represents pagination parameters.
type Paginate struct {
	// Limit represents the number of items returned in the response.
	Limit int
	// Offset represents the starting index of the returned list of items.
	Offset int
}

// PaginatedResponse contains the paginated response data.
type PaginatedResponse struct {
	// Meta contains the response metadata.
	Meta Meta `json:"meta"`
	// Links contains the first, next, previous, and last links for the paginated data.
	Links Links `json:"links"`
	// Data is the paginated data
	Data interface{} `json:"data"`
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
			return []interface{}{}
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

// GetPaginatedResponse accepts the pagination settings and full data list and returns
// the paginated data.
func GetPaginatedResponse(url *url.URL, p Paginate, data interface{}) (*PaginatedResponse, error) {
	if data == nil {
		return nil, fmt.Errorf("invalid data set: data cannot be nil")
	}

	// use lenSlice as error checker. lenSlice returns -1 if the data is not a slice.
	// Paginated data must be a slice
	if lenSlice(data) == -1 {
		return nil, fmt.Errorf("invalid data set: must be a slice")
	}

	if p.Limit < 0 || p.Offset < 0 {
		return nil, fmt.Errorf("invalid negative value for limit or offset")
	}

	return &PaginatedResponse{
		Meta:  getMeta(data),
		Links: getLinks(url, p, data),
		Data:  indexSlice(data, p.Offset, p.Offset+p.Limit),
	}, nil
}

// Meta represents the response metadata.
type Meta struct {
	// Count represents the number of total items the query generated.
	Count int `json:"count"`
}

func getMeta(data interface{}) Meta {
	return Meta{Count: lenSlice(data)}
}

// Links represents the first, next, previous, and last links of the paginated response.
type Links struct {
	// First is the link that represents the start of the paginated data (offset=0).
	First string `json:"first"`
	// Next represents the next page of paginated data.
	Next *string `json:"next"`
	// Previous represents the previous page of paginated data.
	Previous *string `json:"previous"`
	// Last represents the last page of paginated data.
	Last *string `json:"last"`
}

func getFirstLink(url *url.URL, limit, offset int) string {
	firstURL := url
	q := firstURL.Query()
	q.Set("offset", fmt.Sprintf("%d", 0))
	q.Set("limit", fmt.Sprintf("%d", limit))

	firstURL.RawQuery = q.Encode()
	return firstURL.String()
}

func getNextLink(url *url.URL, count, limit, offset int) *string {
	if offset+limit >= count {
		return nil
	}
	nextURL := url
	q := nextURL.Query()
	q.Set("offset", fmt.Sprintf("%d", limit+offset))
	q.Set("limit", fmt.Sprintf("%d", limit))

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
		q.Set("offset", fmt.Sprintf("%d", 0))
	} else {
		q.Set("offset", fmt.Sprintf("%d", offset-limit))
	}

	q.Set("limit", fmt.Sprintf("%d", limit))

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
	q.Set("limit", fmt.Sprintf("%d", limit))

	lastURL.RawQuery = q.Encode()
	last := lastURL.String()

	return &last
}

func getLinks(url *url.URL, p Paginate, data interface{}) Links {
	result := Links{First: getFirstLink(url, p.Limit, p.Offset)}
	count := lenSlice(data)
	if count <= p.Limit && p.Offset == 0 {
		result.Last = &result.First
		return result
	}
	result.Next = getNextLink(url, count, p.Limit, p.Offset)
	result.Previous = getPreviousLink(url, count, p.Limit, p.Offset)
	result.Last = getLastLink(url, count, p.Limit, p.Offset)

	return result
}

// PaginationCtx is a middleware that parses the pagination settings from the url query
// and injects them as a Paginate object in the request context.
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
				errors.BadRequestError(w, fmt.Errorf("invalid limit: %w", err))
				return
			}

			if lim < 0 {
				errors.BadRequestError(w, fmt.Errorf("invalid limt: %d", lim))
				return
			}

			pagination.Limit = lim
		}

		offset := r.URL.Query().Get("offset")
		if offset != "" {
			off, err := strconv.Atoi(offset)
			if err != nil {
				errors.BadRequestError(w, fmt.Errorf("invalid offset: %w", err))
				return
			}

			if off < 0 {
				errors.BadRequestError(w, fmt.Errorf("invalid offset: %d", off))
				return
			}

			pagination.Offset = off
		}

		ctx := context.WithValue(r.Context(), PaginateKey, pagination)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetPagination is a helper function that returns the Paginate
// object stored in the request context.
func GetPagination(ctx context.Context) Paginate {
	return ctx.Value(PaginateKey).(Paginate)
}
