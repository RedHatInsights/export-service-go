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
	"strconv"
)

type paginationKey int

const (
	PaginateKey   paginationKey = iota
	defaultLimit  int           = 100
	defaultOffset int           = 0
	defaultSortBy string        = "created_at"
	defaultDir    string        = "asc"
)

// Pagination represents pagination parameters.
type Paginate struct {
	// Limit represents the number of items returned in the response.
	Limit int
	// Offset represents the starting index of the returned list of items.
	Offset int
	// Sort Direction - asc or desc
	Dir string
	// Sort by - name, created (created_at), or expires
	SortBy string
}

// PaginatedResponse contains the paginated response data.
type PaginatedResponse struct {
	// Meta contains the response metadata.
	Meta Meta `json:"meta"`
	// Links contains the first, next, previous, and last links for the paginated data.
	Links Links       `json:"links"`
	Data  interface{} `json:"data"`
}

// GetPaginatedResponse accepts the pagination settings and full data list and returns
// the paginated data.
func GetPaginatedResponse(url *url.URL, p Paginate, count int64, data interface{}) (*PaginatedResponse, error) {
	if data == nil {
		return nil, fmt.Errorf("invalid data set: data cannot be nil")
	}

	return &PaginatedResponse{
		Meta:  getMeta(count),
		Links: GetLinks(url, p, count, data),
		Data:  data,
	}, nil
}

// Meta represents the response metadata.
type Meta struct {
	// Count represents the number of total items the query generated.
	Count int64 `json:"count"`
}

func getMeta(count int64) Meta {
	// casting int
	return Meta{Count: count}
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
	Last string `json:"last"`
}

func getFirstLink(url *url.URL, limit, offset int) string {
	firstURL := url
	q := firstURL.Query()
	q.Set("offset", fmt.Sprintf("%d", 0))
	q.Set("limit", fmt.Sprintf("%d", limit))

	firstURL.RawQuery = q.Encode()
	return firstURL.String()
}

func getNextLink(url *url.URL, count int64, limit, offset int) *string {
	if int64(offset+limit) >= count {
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

func getPreviousLink(url *url.URL, count int64, limit, offset int) *string {
	if offset <= 0 || int64(offset) >= count {
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

func getLastLink(url *url.URL, count int64, limit, offset int) string {
	lastURL := url
	q := lastURL.Query()
	q.Set("offset", fmt.Sprintf("%d", count-int64(limit)))
	q.Set("limit", fmt.Sprintf("%d", limit))

	lastURL.RawQuery = q.Encode()
	last := lastURL.String()

	return last
}

func GetLinks(url *url.URL, p Paginate, count int64, data interface{}) Links {
	result := Links{First: getFirstLink(url, p.Limit, p.Offset)}
	if count <= int64(p.Limit) && p.Offset == 0 {
		result.Last = result.First
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
			Dir:    defaultDir,
			SortBy: defaultSortBy,
		}
		limit := r.URL.Query().Get("limit")
		if limit != "" {
			lim, err := strconv.Atoi(limit)
			if err != nil {
				BadRequestError(w, fmt.Errorf("invalid limit: %w", err))
				return
			}

			if lim < 0 {
				BadRequestError(w, fmt.Errorf("invalid limt: %d", lim))
				return
			}

			pagination.Limit = lim
		}

		offset := r.URL.Query().Get("offset")
		if offset != "" {
			off, err := strconv.Atoi(offset)
			if err != nil {
				BadRequestError(w, fmt.Errorf("invalid offset: %w", err))
				return
			}

			if off < 0 {
				BadRequestError(w, fmt.Errorf("invalid offset: %d", off))
				return
			}

			pagination.Offset = off
		}

		sort := r.URL.Query().Get("sort")
		if sort != "" {
			switch sort {
			case "name", "expires":
				pagination.SortBy = sort
			case "created":
				pagination.SortBy = "created_at"
			default:
				BadRequestError(w, fmt.Errorf("sort does not match 'name', 'created', or 'expires': %s", sort))
				return
			}
		}

		dir := r.URL.Query().Get("dir")
		if dir != "" {
			switch dir {
			case "asc", "desc":
			default:
				BadRequestError(w, fmt.Errorf("dir does not match 'asc' or 'desc': %s", dir))
				return
			}

			pagination.Dir = dir
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
