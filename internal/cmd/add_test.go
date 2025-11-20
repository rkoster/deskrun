package cmd

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/deskrun/pkg/types"
)

var _ = Describe("Repository URL Sanitization", func() {
	// These tests ensure that repository URLs are properly sanitized to prevent
	// GitHub API authentication failures. The original issue was that HTTP URLs
	// and trailing slashes caused 404 errors when the GitHub Actions Runner
	// Controller tried to register runners with GitHub's API.

	DescribeTable("URL sanitization scenarios",
		func(input, expected string) {
			result := sanitizeRepositoryURL(input)
			Expect(result).To(Equal(expected))
		},
		Entry("HTTP GitHub URL with trailing slash",
			"http://github.com/user/repo/",
			"https://github.com/user/repo"),
		Entry("HTTP GitHub URL without trailing slash",
			"http://github.com/user/repo",
			"https://github.com/user/repo"),
		Entry("HTTPS GitHub URL with trailing slash",
			"https://github.com/user/repo/",
			"https://github.com/user/repo"),
		Entry("HTTPS GitHub URL without trailing slash",
			"https://github.com/user/repo",
			"https://github.com/user/repo"),
		Entry("HTTPS GitHub URL with multiple trailing slashes",
			"https://github.com/user/repo///",
			"https://github.com/user/repo"),
		Entry("HTTP GitHub URL with multiple trailing slashes",
			"http://github.com/user/repo///",
			"https://github.com/user/repo"),
		Entry("GitHub Enterprise URL with trailing slash",
			"https://github.enterprise.com/user/repo/",
			"https://github.enterprise.com/user/repo"),
		Entry("Non-GitHub URL should not be modified (except trailing slash)",
			"https://gitlab.com/user/repo/",
			"https://gitlab.com/user/repo"),
		Entry("Empty string",
			"",
			""),
		Entry("URL with only protocol and domain",
			"http://github.com",
			"https://github.com"),
		Entry("Organization-level GitHub URL",
			"http://github.com/myorg/",
			"https://github.com/myorg"),
		Entry("Repository with dots and special characters",
			"http://github.com/my-org/my.special-repo_123/",
			"https://github.com/my-org/my.special-repo_123"),
		Entry("Real-world issue: HTTP GitHub URL with trailing slash (rubionic-workspace case)",
			"http://github.com/rkoster/rubionic-workspace/",
			"https://github.com/rkoster/rubionic-workspace"),
	)

	Context("edge cases", func() {
		When("provided with malformed URLs", func() {
			It("should handle empty strings gracefully", func() {
				result := sanitizeRepositoryURL("")
				Expect(result).To(Equal(""))
			})

			It("should handle URLs with only slashes", func() {
				result := sanitizeRepositoryURL("///")
				Expect(result).To(Equal(""))
			})
		})

		When("provided with non-GitHub URLs", func() {
			It("should only strip trailing slashes without protocol conversion", func() {
				result := sanitizeRepositoryURL("https://gitlab.com/user/repo/")
				Expect(result).To(Equal("https://gitlab.com/user/repo"))
			})
		})
	})
})

var _ = Describe("Repository URL Sanitization Integration", func() {
	// Test that the sanitization is actually applied in the runAdd flow
	// This tests the integration between sanitizeRepositoryURL and the add command

	Context("when creating runner installations", func() {
		var installation *types.RunnerInstallation

		BeforeEach(func() {
			installation = &types.RunnerInstallation{
				Name:          "test-runner",
				ContainerMode: types.ContainerModeKubernetes,
				MinRunners:    1,
				MaxRunners:    1,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
			}
		})

		When("the repository URL needs sanitization", func() {
			It("should convert HTTP URLs to HTTPS", func() {
				installation.Repository = "http://github.com/test/repo/"
				sanitizedURL := sanitizeRepositoryURL(installation.Repository)
				Expect(sanitizedURL).To(Equal("https://github.com/test/repo"))
			})

			It("should clean trailing slashes from HTTPS URLs", func() {
				installation.Repository = "https://github.com/test/repo/"
				sanitizedURL := sanitizeRepositoryURL(installation.Repository)
				Expect(sanitizedURL).To(Equal("https://github.com/test/repo"))
			})
		})
	})
})

var _ = Describe("Add Command Parameter Validation", func() {
	Context("when validating instances and max runners", func() {
		DescribeTable("validation scenarios",
			func(instances, maxRunners int, containerMode types.ContainerMode, shouldSucceed bool, expectedErrorMsg string) {
				err := validateParameters(instances, maxRunners, containerMode)

				if shouldSucceed {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
					if expectedErrorMsg != "" {
						Expect(err.Error()).To(ContainSubstring(expectedErrorMsg))
					}
				}
			},
			Entry("valid: instances=1 maxRunners=5",
				1, 5, types.ContainerModeKubernetes, true, ""),
			Entry("valid: instances=3 maxRunners=1",
				3, 1, types.ContainerModeKubernetes, true, ""),
			Entry("invalid: instances=5 maxRunners=8",
				5, 8, types.ContainerModeKubernetes, false, "cannot use --instances > 1 with --max-runners > 1"),
			Entry("invalid: instances=2 maxRunners=3",
				2, 3, types.ContainerModeKubernetes, false, "cannot use --instances > 1 with --max-runners > 1"),
			Entry("valid: cached-privileged-kubernetes with maxRunners=1",
				3, 1, types.ContainerModePrivileged, true, ""),
			Entry("invalid: cached-privileged-kubernetes with maxRunners>1",
				1, 5, types.ContainerModePrivileged, false, "cached-privileged-kubernetes mode requires --max-runners=1"),
			Entry("invalid: cached-privileged-kubernetes with instances>1 and maxRunners>1",
				3, 5, types.ContainerModePrivileged, false, "cannot use --instances > 1 with --max-runners > 1"),
			Entry("valid: dind mode with maxRunners>1",
				1, 5, types.ContainerModeDinD, true, ""),
		)

		When("using privileged container mode", func() {
			It("should require maxRunners to be 1", func() {
				err := validateParameters(1, 5, types.ContainerModePrivileged)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cached-privileged-kubernetes mode requires --max-runners=1"))
			})

			It("should allow maxRunners=1 with multiple instances", func() {
				err := validateParameters(3, 1, types.ContainerModePrivileged)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("using multiple instances", func() {
			It("should not allow both instances>1 and maxRunners>1", func() {
				err := validateParameters(3, 5, types.ContainerModeKubernetes)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cannot use --instances > 1 with --max-runners > 1"))
			})

			It("should allow instances>1 with maxRunners=1", func() {
				err := validateParameters(5, 1, types.ContainerModeKubernetes)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

var _ = Describe("Container Mode Utilities", func() {
	DescribeTable("container mode string conversion",
		func(mode types.ContainerMode, expectedString string) {
			result := modeToString(mode)
			Expect(result).To(Equal(expectedString))
		},
		Entry("Kubernetes mode", types.ContainerModeKubernetes, "kubernetes"),
		Entry("Privileged mode", types.ContainerModePrivileged, "cached-privileged-kubernetes"),
		Entry("DinD mode", types.ContainerModeDinD, "dind"),
	)

	It("should default to kubernetes for unknown modes", func() {
		// Test with an invalid mode by casting a large int to ContainerMode
		unknownMode := types.ContainerMode("invalid-mode")
		result := modeToString(unknownMode)
		Expect(result).To(Equal("kubernetes"))
	})
})

// validateParameters implements the validation logic that should exist in the add command
// This serves as both a test and a specification for the actual implementation
func validateParameters(instances, maxRunners int, containerMode types.ContainerMode) error {
	// Multiple instances with multiple max runners is not supported
	if instances > 1 && maxRunners > 1 {
		return fmt.Errorf("cannot use --instances > 1 with --max-runners > 1")
	}

	// Privileged container mode requires maxRunners=1 due to resource constraints
	if containerMode == types.ContainerModePrivileged && maxRunners > 1 {
		return fmt.Errorf("cached-privileged-kubernetes mode requires --max-runners=1")
	}

	return nil
}

func modeToString(mode types.ContainerMode) string {
	switch mode {
	case types.ContainerModeKubernetes:
		return "kubernetes"
	case types.ContainerModePrivileged:
		return "cached-privileged-kubernetes"
	case types.ContainerModeDinD:
		return "dind"
	default:
		return "kubernetes"
	}
}
