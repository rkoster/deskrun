package template_spec

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rkoster/deskrun/internal/runner/template_spec/matchers"
	"github.com/rkoster/deskrun/pkg/templates"
	"github.com/rkoster/deskrun/pkg/types"
)

var _ = Describe("ARC Template Processing", func() {

	var processor *templates.Processor

	BeforeEach(func() {
		processor = templates.NewProcessor()
	})

	Context("Container Mode Variations", func() {

		Context("Kubernetes Mode", func() {
			It("should render basic configuration correctly", func() {
				config := templates.Config{
					Installation: &types.RunnerInstallation{
						Name:          "test-runner",
						Repository:    "https://github.com/test/repo",
						AuthValue:     "test-token",
						ContainerMode: types.ContainerModeKubernetes,
						MinRunners:    1,
						MaxRunners:    3,
						CachePaths:    []types.CachePath{},
					},
					InstanceName: "test-runner",
					InstanceNum:  1,
				}

				actualYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(actualYAML)).To(matchers.MatchYAMLFile("testdata/expected/kubernetes_basic.yaml"))
			})

			It("should handle cache paths correctly", func() {
				config := templates.Config{
					Installation: &types.RunnerInstallation{
						Name:          "test-runner",
						Repository:    "https://github.com/test/repo",
						AuthValue:     "test-token",
						ContainerMode: types.ContainerModeKubernetes,
						MinRunners:    1,
						MaxRunners:    3,
						CachePaths: []types.CachePath{
							{Source: "/var/lib/docker", Target: "/var/lib/docker"},
						},
					},
					InstanceName: "test-runner",
					InstanceNum:  1,
				}

				actualYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(actualYAML)).To(matchers.MatchYAMLFile("testdata/expected/kubernetes_with_caches.yaml"))
			})
		})

		Context("Docker-in-Docker Mode", func() {
			It("should render basic DinD configuration", func() {
				config := templates.Config{
					Installation: &types.RunnerInstallation{
						Name:          "test-runner",
						Repository:    "https://github.com/test/repo",
						AuthValue:     "test-token",
						ContainerMode: types.ContainerModeDinD,
						MinRunners:    1,
						MaxRunners:    3,
						CachePaths:    []types.CachePath{},
					},
					InstanceName: "test-runner",
					InstanceNum:  1,
				}

				actualYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(actualYAML)).To(matchers.MatchYAMLFile("testdata/expected/dind_basic.yaml"))
			})

			It("should use no-permission ServiceAccount for DinD mode", func() {
				config := templates.Config{
					Installation: &types.RunnerInstallation{
						Name:          "test-runner",
						Repository:    "https://github.com/test/repo",
						AuthValue:     "test-token",
						ContainerMode: types.ContainerModeDinD,
						MinRunners:    1,
						MaxRunners:    3,
						CachePaths:    []types.CachePath{},
					},
					InstanceName: "test-runner",
					InstanceNum:  1,
				}

				actualYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())

				// Verify DinD overlay removes manager ServiceAccount resource but keeps no-permission SA
				Expect(string(actualYAML)).To(ContainSubstring("test-runner-gha-rs-no-permission"))
				Expect(string(actualYAML)).To(ContainSubstring("serviceAccountName: test-runner-gha-rs-no-permission"))
			})
		})

		Context("Privileged Mode", func() {
			It("should render with hook extension", func() {
				config := templates.Config{
					Installation: &types.RunnerInstallation{
						Name:          "test-runner",
						Repository:    "https://github.com/test/repo",
						AuthValue:     "test-token",
						ContainerMode: types.ContainerModePrivileged,
						MinRunners:    1,
						MaxRunners:    3,
						CachePaths:    []types.CachePath{},
					},
					InstanceName: "test-runner",
					InstanceNum:  1,
				}

				actualYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(actualYAML)).To(matchers.MatchYAMLFile("testdata/expected/privileged_basic.yaml"))
			})

			It("should handle cache volumes correctly", func() {
				config := templates.Config{
					Installation: &types.RunnerInstallation{
						Name:          "test-runner",
						Repository:    "https://github.com/test/repo",
						AuthValue:     "test-token",
						ContainerMode: types.ContainerModePrivileged,
						MinRunners:    1,
						MaxRunners:    3,
						CachePaths: []types.CachePath{
							{Source: "/var/lib/docker", Target: "/var/lib/docker"},
							{Source: "/nix/store", Target: "/nix/store"},
						},
					},
					InstanceName: "test-runner",
					InstanceNum:  1,
				}

				actualYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(actualYAML)).To(matchers.MatchYAMLFile("testdata/expected/privileged_multi_cache.yaml"))
			})
		})
	})

	Context("ServiceAccount Logic", func() {
		DescribeTable("ServiceAccount creation patterns",
			func(containerMode types.ContainerMode, expectedSARef string) {
				config := templates.Config{
					Installation: &types.RunnerInstallation{
						Name:          "test-runner",
						Repository:    "https://github.com/test/repo",
						AuthValue:     "test-token",
						ContainerMode: containerMode,
						MinRunners:    1,
						MaxRunners:    3,
						CachePaths:    []types.CachePath{},
					},
					InstanceName: "test-runner",
					InstanceNum:  1,
				}

				actualYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())

				// Check AutoscalingRunnerSet ServiceAccount reference
				Expect(string(actualYAML)).To(ContainSubstring(
					"serviceAccountName: test-runner-gha-rs-"+expectedSARef),
					"Expected serviceAccountName reference not found")
			},
			// Upstream Helm charts now use kube-mode service account for kubernetes and privileged modes
			Entry("kubernetes mode uses kube-mode SA", types.ContainerModeKubernetes, "kube-mode"),
			Entry("privileged mode uses kube-mode SA", types.ContainerModePrivileged, "kube-mode"),
			Entry("dind mode uses no-permission SA", types.ContainerModeDinD, "no-permission"),
		)
	})
})
