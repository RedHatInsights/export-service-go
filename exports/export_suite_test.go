package exports_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExports(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Exports Suite")
}
