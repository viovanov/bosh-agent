package applyspec_test

import (
	. "github.com/cloudfoundry/bosh-agent/internal/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry/bosh-agent/internal/github.com/onsi/gomega"

	"testing"
)

func TestApplyspec(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Applyspec Suite")
}
