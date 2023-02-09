package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	"github.com/redhatinsights/export-service-go/middleware"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Handler", func() {
	Describe("Test getLinks function", func() {
		It("should return the proper Links struct", func() {
			count := 100
			data := make([]int, count)

			// construct the url
			url := &url.URL{
				Path: "/test",
			}

			links := middleware.GetLinks(url, middleware.Paginate{Limit: 10, Offset: 20}, int64(count), data)

			expectedFirst := "/test?limit=10&offset=0"
			Expect(links.First).To(Equal(expectedFirst))

			expectedNext := "/test?limit=10&offset=30"
			expectedPrevious := "/test?limit=10&offset=10"
			expectedLast := "/test?limit=10&offset=90"

			Expect(links.Next).To(Equal(&expectedNext))
			Expect(links.Previous).To(Equal(&expectedPrevious))
			Expect(links.Last).To(Equal(expectedLast))

			Expect(links).To(Equal(middleware.Links{
				First:    expectedFirst,
				Next:     &expectedNext,
				Previous: &expectedPrevious,
				Last:     expectedLast,
			}))
		})
	})

	DescribeTable("Test PaginationCtx function",
		func(useDefaults bool, limit, offset string, expectedStatus int) {
			var requestString string
			if useDefaults {
				requestString = "/test"
			} else {
				requestString = fmt.Sprintf("/test?limit=%s&offset=%s", limit, offset)
			}
			req, err := http.NewRequest("GET", requestString, nil)
			Expect(err).To(BeNil())

			expectedPaginate := middleware.Paginate{}
			if expectedStatus == http.StatusOK {
				l, err := strconv.Atoi(limit)
				Expect(err).To(BeNil())
				o, err := strconv.Atoi(offset)
				Expect(err).To(BeNil())

				expectedPaginate = middleware.Paginate{
					Limit:  l,
					Offset: o,
					SortBy: "created_at",
					Dir:    "asc",
				}
			}

			handlerCalled := false

			rr := httptest.NewRecorder()
			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				pagination := r.Context().Value(middleware.PaginateKey).(middleware.Paginate)
				Expect(pagination).To(Equal(expectedPaginate))

				handlerCalled = true
			})

			router := chi.NewRouter()
			router.Route("/", func(sub chi.Router) {
				sub.Use(middleware.PaginationCtx)
				sub.Get("/test", applicationHandler)
			})

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(expectedStatus))

			// Handler should not be called if an error is expected
			// The middleware would pass a bad context
			Expect(handlerCalled).To(Equal(expectedStatus == http.StatusOK))
		},
		Entry("Use default values of 100 Limit and 0 Offset", true, "100", "0", http.StatusOK),
		Entry("Use passed values", false, "10", "20", http.StatusOK),
		Entry("Pass negative values", false, "-10", "-20", http.StatusBadRequest),
		Entry("Pass zero values", false, "0", "0", http.StatusOK),
		Entry("Pass non-integer values", false, "a", "b", http.StatusBadRequest),
	)
})
