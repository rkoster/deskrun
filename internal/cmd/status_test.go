package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Status Command Helpers", func() {
	Describe("formatAge", func() {
		DescribeTable("age formatting scenarios",
			func(input, expected string) {
				result := formatAge(input)
				Expect(result).To(Equal(expected))
			},
			// Ages >= 3 characters are returned unchanged
			Entry("3 character age - unchanged", "10h", "10h"),
			Entry("4 character age - unchanged", "100h", "100h"),
			Entry("5+ character age - unchanged", "1000d", "1000d"),

			// 2-character ages get a leading zero
			Entry("2 character age - single zero", "5s", "05s"),
			Entry("2 character age - hours", "5h", "05h"),
			Entry("2 character age - days", "5d", "05d"),
			Entry("2 character age - 9s", "9s", "09s"),

			// 1-character ages get two leading zeros
			Entry("1 character age", "5", "005"),
			Entry("1 character age - 0", "0", "000"),
			Entry("1 character age - 9", "9", "009"),

			// Edge cases
			Entry("empty string", "", ""),
		)

		Context("with real-world age values", func() {
			It("should format single-digit seconds", func() {
				Expect(formatAge("5s")).To(Equal("05s"))
			})

			It("should format double-digit hours", func() {
				Expect(formatAge("23h")).To(Equal("23h"))
			})

			It("should not modify already-formatted ages", func() {
				Expect(formatAge("100d")).To(Equal("100d"))
			})
		})
	})

	Describe("extractHierarchyInfo", func() {
		DescribeTable("hierarchy extraction scenarios",
			func(input, expectedPrefix, expectedName string) {
				prefix, name := extractHierarchyInfo(input)
				Expect(prefix).To(Equal(expectedPrefix))
				Expect(name).To(Equal(expectedName))
			},
			// No hierarchy (no leading spaces)
			Entry("simple name - no hierarchy",
				"rubionic-workspace-1",
				"", "rubionic-workspace-1"),
			Entry("name with dashes - no hierarchy",
				"rubionic-workspace-1-gha-rs-kube-mode",
				"", "rubionic-workspace-1-gha-rs-kube-mode"),

			// Single-level hierarchy with L marker
			Entry("L marker - single level",
				" L rubionic-workspace-1-listener",
				"L ", "rubionic-workspace-1-listener"),

			// Double-level hierarchy with L.. marker (should convert to "  L ")
			Entry("L.. marker - double level",
				" L.. rubionic-workspace-1-runner-6mckt",
				"  L ", "rubionic-workspace-1-runner-6mckt"),

			// Space-based hierarchy without markers
			Entry("single space - no L marker",
				" resource-name",
				"L ", "resource-name"),
			Entry("double space - no L marker",
				"  resource-name",
				"  L ", "resource-name"),
			Entry("triple space - no L marker",
				"   resource-name",
				"  L ", "resource-name"),

			// Edge cases with L marker but no name
			Entry("L marker only - no name",
				" L",
				"", " L"),

			// Multiple levels beyond L..
			Entry("L... marker (3 dots) - should use L... prefix",
				" L... resource-name",
				"L... ", "resource-name"),
		)

		Context("with real-world resource names from kapp", func() {
			It("should extract root-level resources", func() {
				prefix, name := extractHierarchyInfo("rubionic-workspace-1-gha-rs-kube-mode")
				Expect(prefix).To(Equal(""))
				Expect(name).To(Equal("rubionic-workspace-1-gha-rs-kube-mode"))
			})

			It("should extract L hierarchy level", func() {
				prefix, name := extractHierarchyInfo(" L rubionic-workspace-1-6cd58d58-listener")
				Expect(prefix).To(Equal("L "))
				Expect(name).To(Equal("rubionic-workspace-1-6cd58d58-listener"))
			})

			It("should convert L.. to proper indent", func() {
				prefix, name := extractHierarchyInfo(" L.. rubionic-workspace-1-scc6w-runner-rshjm")
				Expect(prefix).To(Equal("  L "))
				Expect(name).To(Equal("rubionic-workspace-1-scc6w-runner-rshjm"))
			})
		})

		Context("with malformed inputs", func() {
			It("should handle leading spaces without L marker", func() {
				prefix, name := extractHierarchyInfo(" some-resource")
				Expect(prefix).To(Equal("L "))
				Expect(name).To(Equal("some-resource"))
			})

			It("should handle multiple leading spaces", func() {
				prefix, name := extractHierarchyInfo("   some-resource")
				Expect(prefix).To(Equal("  L "))
				Expect(name).To(Equal("some-resource"))
			})

			It("should handle empty string", func() {
				prefix, name := extractHierarchyInfo("")
				Expect(prefix).To(Equal(""))
				Expect(name).To(Equal(""))
			})
		})
	})
})
