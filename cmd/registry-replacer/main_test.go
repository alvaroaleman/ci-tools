package main

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/ci-tools/pkg/api"
	"github.com/openshift/ci-tools/pkg/api/ocpbuilddata"
	"github.com/openshift/ci-tools/pkg/config"
	"github.com/openshift/ci-tools/pkg/github"
	"github.com/openshift/ci-tools/pkg/testhelper"
)

func TestReplacer(t *testing.T) {
	majorMinor := ocpbuilddata.MajorMinor{Major: "4", Minor: "6"}
	testCases := []struct {
		name                               string
		config                             *api.ReleaseBuildConfiguration
		pruneUnusedReplacementsEnabled     bool
		pruneOCPBuilderReplacementsEnabled bool
		ensureCorrectPromotionDockerfile   bool
		promotionTargetToDockerfileMapping map[string]string
		files                              map[string][]byte
		expectWrite                        bool
	}{
		{
			name: "No dockerfile, does nothing",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{}},
			},
		},
		{
			name: "Default to dockerfile",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{}},
			},
			files:       map[string][]byte{"Dockerfile": []byte("FROM registry.svc.ci.openshift.org/org/repo:tag")},
			expectWrite: true,
		},
		{
			name: "Existing base_image is not overwritten",
			config: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BaseImages: map[string]api.ImageStreamTagReference{
						"org_repo_tag": {Namespace: "other_org", Name: "other_repo", Tag: "other_tag"},
					},
				},
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{}},
			},
			files:       map[string][]byte{"Dockerfile": []byte("FROM registry.svc.ci.openshift.org/org/repo:tag")},
			expectWrite: true,
		},
		{
			name: "ContextDir is respected",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{ContextDir: "my-dir"}}},
			},
			files:       map[string][]byte{"my-dir/Dockerfile": []byte("FROM registry.svc.ci.openshift.org/org/repo:tag")},
			expectWrite: true,
		},
		{
			name: "Existing replace is respected",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
					Inputs: map[string]api.ImageBuildInputs{"some-image": {As: []string{"registry.svc.ci.openshift.org/org/repo:tag"}}}}},
				},
			},
			files: map[string][]byte{"Dockerfile": []byte("FROM registry.svc.ci.openshift.org/org/repo:tag")},
		},
		{
			name: "Replaces with tag",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						DockerfilePath: "dockerfile",
					},
				}},
			},
			files:       map[string][]byte{"dockerfile": []byte("FROM registry.svc.ci.openshift.org/org/repo:tag")},
			expectWrite: true,
		},
		{
			name: "Replaces without tag",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						DockerfilePath: "dockerfile",
					},
				}},
			},
			files:       map[string][]byte{"dockerfile": []byte("FROM registry.svc.ci.openshift.org/org/repo")},
			expectWrite: true,
		},
		{
			name: "Different registry, does nothing",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						DockerfilePath: "dockerfile",
					},
				}},
			},
			files: map[string][]byte{"dockerfile": []byte("FROM registry.svc2.ci.openshift.org/org/repo")},
		},
		{
			name: "Build APIs replacement is executed first",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					From: "base",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						DockerfilePath: "dockerfile",
					},
				}},
			},
			files:       map[string][]byte{"dockerfile": []byte("FROM registry.svc.ci.openshift.org/org/repo as repo\nFROM registry.svc.ci.openshift.org/org/repo2")},
			expectWrite: true,
		},
		{
			name: "No pruning on empty Dockerfile",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					From: "base",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						DockerfilePath: "dockerfile",
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"builder"}},
						},
					},
				}},
			},
			pruneUnusedReplacementsEnabled: true,
		},
		{
			name: "OCP builder pruning happens",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"ocp/builder:something"}},
						},
					},
				}},
			},
			pruneOCPBuilderReplacementsEnabled: true,
			expectWrite:                        true,
		},
		{
			name: "Dockerfile gets fixed up",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"ocp/builder:something"}},
						},
					},
					To: "promotionTarget",
				}},
				PromotionConfiguration: &api.PromotionConfiguration{Namespace: "ocp", Name: majorMinor.String()},
				Metadata:               api.Metadata{Branch: "master"},
			},
			ensureCorrectPromotionDockerfile:   true,
			promotionTargetToDockerfileMapping: map[string]string{fmt.Sprintf("registry.svc.ci.openshift.org/ocp/%s:promotionTarget", majorMinor.String()): "Dockerfile.rhel"},
			expectWrite:                        true,
		},
		{
			name: "Config for non-master branch is ignored",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"ocp/builder:something"}},
						},
					},
					To: "promotionTarget",
				}},
				PromotionConfiguration: &api.PromotionConfiguration{Namespace: "ocp", Name: majorMinor.String()},
			},
			ensureCorrectPromotionDockerfile:   true,
			promotionTargetToDockerfileMapping: map[string]string{fmt.Sprintf("registry.svc.ci.openshift.org/ocp/%s:promotionTarget", majorMinor.String()): "Dockerfile.rhel"},
		},
		{
			name: "Dockerfile is correct, nothing to do",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						DockerfilePath: "Dockerfile.rhel",
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"ocp/builder:something"}},
						},
					},
					To: "promotionTarget",
				}},
				PromotionConfiguration: &api.PromotionConfiguration{Namespace: "ocp", Name: majorMinor.String()},
				Metadata:               api.Metadata{Branch: "master"},
			},
			ensureCorrectPromotionDockerfile:   true,
			promotionTargetToDockerfileMapping: map[string]string{fmt.Sprintf("registry.svc.ci.openshift.org/ocp/%s:promotionTarget", majorMinor.String()): "Dockerfile.rhel"},
		},
		{
			name: "Dockerfile+Context dir gets fixed up",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						ContextDir:     "some-dir",
						DockerfilePath: "Dockerfile.rhel",
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"ocp/builder:something"}},
						},
					},
					To: "promotionTarget",
				}},
				PromotionConfiguration: &api.PromotionConfiguration{Namespace: "ocp", Name: majorMinor.String()},
				Metadata:               api.Metadata{Branch: "master"},
			},
			ensureCorrectPromotionDockerfile:   true,
			promotionTargetToDockerfileMapping: map[string]string{fmt.Sprintf("registry.svc.ci.openshift.org/ocp/%s:promotionTarget", majorMinor.String()): "other_dir/Dockerfile.rhel"},
			expectWrite:                        true,
		},
		{
			name: "Dockerfile+Context dir is correct, nothing to do",
			config: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						ContextDir:     "some_dir",
						DockerfilePath: "Dockerfile.rhel",
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"ocp/builder:something"}},
						},
					},
					To: "promotionTarget",
				}},
				PromotionConfiguration: &api.PromotionConfiguration{Namespace: "ocp", Name: majorMinor.String()},
				Metadata:               api.Metadata{Branch: "master"},
			},
			ensureCorrectPromotionDockerfile:   true,
			promotionTargetToDockerfileMapping: map[string]string{fmt.Sprintf("registry.svc.ci.openshift.org/ocp/%s:promotionTarget", majorMinor.String()): "some_dir/Dockerfile.rhel"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fakeWriter := &fakeWriter{}
			if err := replacer(
				fakeGithubFileGetterFactory(tc.files),
				fakeWriter.Write,
				tc.pruneUnusedReplacementsEnabled,
				tc.pruneOCPBuilderReplacementsEnabled,
				tc.ensureCorrectPromotionDockerfile,
				tc.promotionTargetToDockerfileMapping,
				majorMinor,
			)(tc.config, &config.Info{}); err != nil {
				t.Errorf("replacer failed: %v", err)
			}
			if (fakeWriter.data != nil) != tc.expectWrite {
				t.Fatalf("expected write: %t, got data: %s", tc.expectWrite, string(fakeWriter.data))
			}

			if !tc.expectWrite {
				return
			}

			testhelper.CompareWithFixture(t, fakeWriter.data)
		})
	}
}

type fakeWriter struct {
	data []byte
}

func (fw *fakeWriter) Write(data []byte) error {
	fw.data = data
	return nil
}

func fakeGithubFileGetterFactory(data map[string][]byte) func(string, string, string) github.FileGetter {
	return func(_, _, _ string) github.FileGetter {
		return func(path string) ([]byte, error) {
			return data[path], nil
		}
	}
}

func TestExtractReplacementCandidatesFromDockerfile(t *testing.T) {
	testCases := []struct {
		name           string
		in             string
		expectedResult sets.String
	}{
		{
			name:           "Simple",
			in:             "FROM capetown/center:1",
			expectedResult: sets.NewString("capetown/center:1"),
		},
		{
			name:           "Copy --from",
			in:             "FROM centos:7\nCOPY --from=builder /go/src/github.com/code-ready/crc /opt/crc",
			expectedResult: sets.NewString("centos:7", "builder"),
		},
		{
			name: "Multiple from and copy --from",
			in: `FROM registry.svc.ci.openshift.org/openshift/release:golang-1.13 AS builder
WORKDIR /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver
COPY . .
RUN make

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base
# Get mkfs & blkid
RUN yum update -y && \
    yum install --setopt=tsflags=nodocs -y e2fsprogs xfsprogs util-linux && \
    yum clean all && rm -rf /var/cache/yum/*
COPY --from=builder /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver/bin/aws-ebs-csi-driver /usr/bin/
ENTRYPOINT ["/usr/bin/aws-ebs-csi-driver"]`,
			expectedResult: sets.NewString("registry.svc.ci.openshift.org/openshift/release:golang-1.13", "registry.svc.ci.openshift.org/openshift/origin-v4.0:base"),
		},
		{
			name: "Unrelated directives",
			in:   "RUN somestuff\n\n\n ENV var=val",
		},
		{
			name: "Defunct from",
			in:   "from\n\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := extractReplacementCandidatesFromDockerfile([]byte(tc.in))
			if err != nil {
				t.Fatalf("error: %v", err)
			}

			if !result.Equal(tc.expectedResult) {
				t.Errorf("result does not match expected, wanted: %v, got: %v", tc.expectedResult.List(), result.List())
			}
		})
	}
}

func TestPruneUnusedReplacements(t *testing.T) {
	testCases := []struct {
		name            string
		in              *api.ReleaseBuildConfiguration
		allSourceImages sets.String
		expected        *api.ReleaseBuildConfiguration
	}{
		{
			name: "All replacements are valid",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image"}},
						},
					},
				}},
			},
			allSourceImages: sets.NewString("some-image"),
			expected: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image"}},
						},
					},
				}},
			},
		},
		{
			name: "One As gets removed",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image", "superfluous"}},
						},
					}},
				},
			},
			allSourceImages: sets.NewString("some-image"),
			expected: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image"}},
						},
					}},
				},
			},
		},
		{
			name: "One input is empty and gets removed",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder":   {As: []string{"some-image"}},
							"architect": {As: []string{"who-needs-this"}},
						},
					}},
				},
			},
			allSourceImages: sets.NewString("some-image"),
			expected: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image"}},
						},
					}},
				},
			},
		},
		{
			name: "Whole image is empty and gets removed",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image"}},
						},
					}},
				},
			},
			expected: &api.ReleaseBuildConfiguration{},
		},
		{
			name: "Whole image is empty but has paths directives",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image"}, Paths: []api.ImageSourcePath{{}}},
						},
					}},
				},
			},
			expected: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {Paths: []api.ImageSourcePath{{}}},
						},
					}},
				},
			},
		},
		{
			name: "Whole image is empty but has from",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					From: "some-where",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image"}},
						},
					}},
				},
			},
			expected: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					From:                             "some-where",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{}},
				},
			},
		},
		{
			name: "Whole image is empty but has to",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					To: "some-when",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"builder": {As: []string{"some-image"}},
						},
					}},
				},
			},
			expected: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					To: "some-when",
				}},
			},
		},
		{
			name:            "cnc",
			allSourceImages: sets.NewString("scratch", "centos:7", "builder"),
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					From: "base",
					To:   "snc",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						DockerfilePath: "images/openshift-ci/Dockerfile",
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"builder"}},
						},
					}},
				},
			},
			expected: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					From: "base",
					To:   "snc",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						DockerfilePath: "images/openshift-ci/Dockerfile",
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"builder"}},
						},
					}},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := pruneUnusedReplacements(tc.in, tc.allSourceImages); err != nil {
				t.Fatalf("pruneUnusedReplacements failed: %v", err)
			}
			if diff := cmp.Diff(tc.in, tc.expected, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("result differs from expected: %s", diff)
			}
		})
	}
}

func TestPruneOCPBuilderReplacements(t *testing.T) {
	testCases := []struct {
		name     string
		in       *api.ReleaseBuildConfiguration
		expected *api.ReleaseBuildConfiguration
	}{
		{
			name: "Non-OCP builder replacement is left",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"builder"}},
						},
					}},
				},
			},
			expected: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"builder"}},
						},
					}},
				},
			},
		},
		{
			name: "OCP builder replacement is removed",
			in: &api.ReleaseBuildConfiguration{
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"root": {As: []string{"ocp/builder:blub"}},
						},
					}},
				},
			},
			expected: &api.ReleaseBuildConfiguration{},
		},
		{
			name: "OCP builder that directly references api.ci is left",
			in: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BaseImages: map[string]api.ImageStreamTagReference{"ocp_builder_go-1.13": {
						Namespace: "ocp",
						Name:      "builder",
						Tag:       "go-1.13",
					}},
				},
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"ocp_builder_go-1.13": {As: []string{"registry.svc.ci.openshift.org/ocp/builder:go-1.13"}},
						},
					}},
				},
			},
			expected: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BaseImages: map[string]api.ImageStreamTagReference{"ocp_builder_go-1.13": {
						Namespace: "ocp",
						Name:      "builder",
						Tag:       "go-1.13",
					}},
				},
				Images: []api.ProjectDirectoryImageBuildStepConfiguration{{
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						Inputs: map[string]api.ImageBuildInputs{
							"ocp_builder_go-1.13": {As: []string{"registry.svc.ci.openshift.org/ocp/builder:go-1.13"}},
						},
					}},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := pruneOCPBuilderReplacements(tc.in); err != nil {
				t.Fatalf("pruning failed: %v", err)
			}

			if diff := cmp.Diff(tc.in, tc.expected); diff != "" {
				t.Errorf("actual differs from expected: %s", diff)
			}
		})
	}
}
