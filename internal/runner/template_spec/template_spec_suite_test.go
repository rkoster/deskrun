package template_spec

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTemplateSpec(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Template Spec Suite")
}
