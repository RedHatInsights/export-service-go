package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"

	"github.com/redhatinsights/export-service-go/middleware"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Handler", func() {
	Describe("Test getLinks function", func() {
		It("should return the proper Links struct", func() {
			req, err := http.NewRequest("GET", "/test", nil)
			Expect(err).To(BeNil())

			data := make([]int, 100)

			rr := httptest.NewRecorder()
			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				// (url *url.URL, p Paginate, data interface{})
				links := middleware.GetLinks(r.URL, middleware.Paginate{Limit: 10, Offset: 20}, data)

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

			router := chi.NewRouter()
			router.Route("/", func(sub chi.Router) {
				sub.Get("/test", applicationHandler)
			})
			router.ServeHTTP(rr, req)
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

	// Test the GetPaginatedResponse function
	DescribeTable("Test that the proper PaginatedResponse is returned from GetPaginatedResponse",
		func(limit, offset int, data interface{}, expectedCount int, expectedFirst, expectedLast, expectedNext, expectedPrevious string, expectedData interface{}, expectedError error) {
			// Make a paginate struct
			paginate := middleware.Paginate{
				Limit:  limit,
				Offset: offset,
			}

			var expectedResponse *middleware.PaginatedResponse
			if expectedError == nil {
				var expectedNextPtr *string
				var expectedPreviousPtr *string
				if expectedPrevious != "" {
					expectedPreviousPtr = &expectedPrevious
				}
				if expectedNext != "" {
					expectedNextPtr = &expectedNext
				}

				// Make PaginatedResponse with expected values
				expectedResponse = &middleware.PaginatedResponse{
					Meta: middleware.Meta{
						Count: expectedCount,
					},
					Links: middleware.Links{
						First:    expectedFirst,
						Last:     expectedLast,
						Next:     expectedNextPtr,
						Previous: expectedPreviousPtr,
					},
					Data: expectedData,
				}
			}

			req, err := http.NewRequest("GET", "/test", nil)
			Expect(err).ToNot(HaveOccurred())

			rr := httptest.NewRecorder()

			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				resp, err := middleware.GetPaginatedResponse(r.URL, paginate, data)

				if expectedError != nil {
					Expect(err.Error()).To(Equal(expectedError.Error()))
				} else {
					Expect(err).To(BeNil())
				}

				Expect(resp).To(Equal(expectedResponse))
			})

			router := chi.NewRouter()
			router.Route("/", func(sub chi.Router) {
				sub.Get("/test", applicationHandler)
			})

			router.ServeHTTP(rr, req)
		},
		Entry("Null data",
			10,
			0,
			nil,
			0,
			"",
			"",
			"",
			"",
			nil,
			fmt.Errorf("invalid data set: data cannot be nil"),
		),
		Entry("Empty data",
			10,
			0,
			[]string{},
			0,
			"/test?limit=10&offset=0",
			"/test?limit=10&offset=0",
			"",
			"",
			[]interface{}{},
			nil,
		),
		Entry("Data with 1 item",
			10,
			0,
			[]string{"test"},
			1,
			"/test?limit=10&offset=0",
			"/test?limit=10&offset=0",
			"",
			"",
			[]string{"test"},
			nil,
		),
		Entry("Data with 10 items",
			10,
			0,
			[]string{"test", "test", "test", "test", "test", "test", "test", "test", "test", "test"},
			10,
			"/test?limit=10&offset=0",
			"/test?limit=10&offset=0",
			"",
			"",
			[]string{"test", "test", "test", "test", "test", "test", "test", "test", "test", "test"},
			nil,
		),
		Entry("Data with 11 items",
			10,
			0,
			[]string{"test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test"},
			11,
			"/test?limit=10&offset=0",
			"/test?limit=10&offset=1",
			"/test?limit=10&offset=10",
			"",
			[]string{"test", "test", "test", "test", "test", "test", "test", "test", "test", "test"},
			nil,
		),
		Entry("Data with 12 items",
			10,
			0,
			[]string{"test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test"},
			12,
			"/test?limit=10&offset=0",
			"/test?limit=10&offset=2",
			"/test?limit=10&offset=10",
			"",
			[]string{"test", "test", "test", "test", "test", "test", "test", "test", "test", "test"},
			nil,
		),
		Entry("Data with 20 items",
			10,
			0,
			[]string{"test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test", "test"},
			20,
			"/test?limit=10&offset=0",
			"/test?limit=10&offset=10",
			"/test?limit=10&offset=10",
			"",
			[]string{"test", "test", "test", "test", "test", "test", "test", "test", "test", "test"},
			nil,
		),
		Entry("Data with 20 items and offset 10",
			10,
			10,
			[]string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen", "twenty"},
			20,
			"/test?limit=10&offset=0",
			"/test?limit=10&offset=10",
			"",
			"/test?limit=10&offset=0",
			[]string{"eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen", "twenty"},
			nil,
		),
		Entry("Data with offset greater than data length",
			10,
			100,
			[]string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen", "twenty"},
			20,
			"/test?limit=10&offset=0",
			"/test?limit=10&offset=10",
			"",
			"/test?limit=10&offset=90",
			[]interface{}{},
			nil,
		),
		Entry("Negative offset and limit",
			-10,
			-10,
			[]string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen", "twenty"},
			20,
			"",
			"",
			"",
			"",
			[]interface{}{},
			fmt.Errorf("invalid negative value for limit or offset"),
		),
	)
})
