package defaults

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"

	"github.com/openshift/ci-tools/pkg/api"
)

func addCloneRefs(cfg *api.SourceStepConfiguration) *api.SourceStepConfiguration {
	cfg.ClonerefsImage = api.ImageStreamTagReference{Namespace: "ci", Name: "clonerefs", Tag: "latest"}
	cfg.ClonerefsPath = "/app/prow/cmd/clonerefs/app.binary.runfiles/io_k8s_test_infra/prow/cmd/clonerefs/linux_amd64_pure_stripped/app.binary"
	return cfg
}

func TestStepConfigsForBuild(t *testing.T) {
	var testCases = []struct {
		name     string
		input    *api.ReleaseBuildConfiguration
		jobSpec  *api.JobSpec
		output   []api.StepConfiguration
		readFile readFile
	}{
		{
			name: "minimal information provided",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
				},
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "base-1",
						Name:      "repo-test-base",
						Tag:       "manual",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}},
		},
		{
			name: "minimal information provided with build_root_image from repo",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						FromRepository: true,
					},
				},
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "stream-namespace",
						Name:      "stream-name",
						Tag:       "stream-tag",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}},
			readFile: func(filename string) ([]byte, error) {
				if filename != ".build_root_image" {
					return nil, fmt.Errorf("expected '.build_root_image' as file for the build_root_image, got %s", filename)
				}
				return []byte("stream-namespace/stream-name:stream-tag"), nil
			},
		},
		{
			name: "minimal information provided with build_root_image from repo, tag gets defaulted",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						FromRepository: true,
					},
				},
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "stream-namespace",
						Name:      "stream-name",
						Tag:       "latest",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}},
			readFile: func(filename string) ([]byte, error) {
				if filename != ".build_root_image" {
					return nil, fmt.Errorf("expected '.build_root_image' as file for the build_root_image, got %s", filename)
				}
				return []byte("stream-namespace/stream-name"), nil
			},
		},
		{
			name: "binary build requested",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
				},
				BinaryBuildCommands: "hi",
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "base-1",
						Name:      "repo-test-base",
						Tag:       "manual",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}, {
				PipelineImageCacheStepConfiguration: &api.PipelineImageCacheStepConfiguration{
					From:     api.PipelineImageStreamTagReferenceSource,
					To:       api.PipelineImageStreamTagReferenceBinaries,
					Commands: "hi",
				},
			}},
		},
		{
			name: "binary and rpm build requested",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
				},
				BinaryBuildCommands: "hi",
				RpmBuildCommands:    "hello",
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "base-1",
						Name:      "repo-test-base",
						Tag:       "manual",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}, {
				PipelineImageCacheStepConfiguration: &api.PipelineImageCacheStepConfiguration{
					From:     api.PipelineImageStreamTagReferenceSource,
					To:       api.PipelineImageStreamTagReferenceBinaries,
					Commands: "hi",
				},
			}, {
				PipelineImageCacheStepConfiguration: &api.PipelineImageCacheStepConfiguration{
					From:     api.PipelineImageStreamTagReferenceBinaries,
					To:       api.PipelineImageStreamTagReferenceRPMs,
					Commands: "hello; ln -s $( pwd )/_output/local/releases/rpms/ /srv/repo",
				},
			}, {
				RPMServeStepConfiguration: &api.RPMServeStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRPMs,
				},
			}},
		},
		{
			name: "rpm but not binary build requested",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
				},
				RpmBuildCommands: "hello",
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "base-1",
						Name:      "repo-test-base",
						Tag:       "manual",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}, {
				PipelineImageCacheStepConfiguration: &api.PipelineImageCacheStepConfiguration{
					From:     api.PipelineImageStreamTagReferenceSource,
					To:       api.PipelineImageStreamTagReferenceRPMs,
					Commands: "hello; ln -s $( pwd )/_output/local/releases/rpms/ /srv/repo",
				},
			}, {
				RPMServeStepConfiguration: &api.RPMServeStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRPMs,
				},
			}},
		},
		{
			name: "rpm with custom output but not binary build requested",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
				},
				RpmBuildLocation: "testing",
				RpmBuildCommands: "hello",
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "base-1",
						Name:      "repo-test-base",
						Tag:       "manual",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}, {
				PipelineImageCacheStepConfiguration: &api.PipelineImageCacheStepConfiguration{
					From:     api.PipelineImageStreamTagReferenceSource,
					To:       api.PipelineImageStreamTagReferenceRPMs,
					Commands: "hello; ln -s $( pwd )/testing /srv/repo",
				},
			}, {
				RPMServeStepConfiguration: &api.RPMServeStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRPMs,
				},
			}},
		},
		{
			name: "explicit base image requested",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
					BaseImages: map[string]api.ImageStreamTagReference{
						"name": {
							Namespace: "namespace",
							Name:      "name",
							Tag:       "tag",
						},
					},
				},
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "base-1",
						Name:      "repo-test-base",
						Tag:       "manual",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "namespace",
						Name:      "name",
						Tag:       "tag",
						As:        "name",
					},
					To: api.PipelineImageStreamTagReference("name"),
				},
			}},
		},
		{
			name: "implicit base image from release configuration",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
					ReleaseTagConfiguration: &api.ReleaseTagConfiguration{
						Namespace: "test",
						Name:      "other",
					},
					BaseImages: map[string]api.ImageStreamTagReference{
						"name": {
							Tag: "tag",
						},
					},
				},
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{
				{
					InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
						BaseImage: api.ImageStreamTagReference{
							Namespace: "base-1",
							Name:      "repo-test-base",
							Tag:       "manual",
						},
						To: api.PipelineImageStreamTagReferenceRoot,
					},
				},
				{
					SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
						From: api.PipelineImageStreamTagReferenceRoot,
						To:   api.PipelineImageStreamTagReferenceSource,
					}),
				},
				{
					InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
						BaseImage: api.ImageStreamTagReference{
							Namespace: "test",
							Name:      "other",
							Tag:       "tag",
							As:        "name",
						},
						To: api.PipelineImageStreamTagReference("name"),
					},
				},
				{
					ReleaseImagesTagStepConfiguration: &api.ReleaseTagConfiguration{
						Namespace: "test",
						Name:      "other",
					},
				},
			},
		},
		{
			name: "rpm base image requested",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
					BaseRPMImages: map[string]api.ImageStreamTagReference{
						"name": {
							Namespace: "namespace",
							Name:      "name",
							Tag:       "tag",
						},
					},
				},
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				SourceStepConfiguration: addCloneRefs(&api.SourceStepConfiguration{
					From: api.PipelineImageStreamTagReferenceRoot,
					To:   api.PipelineImageStreamTagReferenceSource,
				}),
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "base-1",
						Name:      "repo-test-base",
						Tag:       "manual",
					},
					To: api.PipelineImageStreamTagReferenceRoot,
				},
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{
						Namespace: "namespace",
						Name:      "name",
						Tag:       "tag",
						As:        "name",
					},
					To: api.PipelineImageStreamTagReference("name-without-rpms"),
				},
			}, {
				RPMImageInjectionStepConfiguration: &api.RPMImageInjectionStepConfiguration{
					From: api.PipelineImageStreamTagReference("name-without-rpms"),
					To:   api.PipelineImageStreamTagReference("name"),
				},
			}},
		},
		{
			name: "including an operator bundle creates the bundle-sub and the index-gen and index images",
			input: &api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					BuildRootImage: &api.BuildRootImageConfiguration{
						ImageStreamTagReference: &api.ImageStreamTagReference{Tag: "manual"},
					},
				},
				Operator: &api.OperatorStepConfiguration{
					Bundles: []api.Bundle{{
						ContextDir:     "manifests/olm",
						DockerfilePath: "bundle.Dockerfile",
					}},
					Substitutions: []api.PullSpecSubstitution{{
						PullSpec: "quay.io/origin/oc",
						With:     "pipeline:oc",
					}},
				},
			},
			jobSpec: &api.JobSpec{
				JobSpec: downwardapi.JobSpec{
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					},
				},
				BaseNamespace: "base-1",
			},
			output: []api.StepConfiguration{{
				BundleSourceStepConfiguration: &api.BundleSourceStepConfiguration{
					Substitutions: []api.PullSpecSubstitution{{
						PullSpec: "quay.io/origin/oc",
						With:     "pipeline:oc",
					}},
				},
			}, {
				IndexGeneratorStepConfiguration: &api.IndexGeneratorStepConfiguration{
					To:            "ci-index-gen",
					OperatorIndex: []string{"ci-bundle0"},
				},
			}, {
				InputImageTagStepConfiguration: &api.InputImageTagStepConfiguration{
					BaseImage: api.ImageStreamTagReference{Namespace: "base-1", Name: "repo-test-base", Tag: "manual"},
					To:        "root",
				},
			}, {
				ProjectDirectoryImageBuildStepConfiguration: &api.ProjectDirectoryImageBuildStepConfiguration{
					To:                               "ci-index",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{DockerfilePath: "index.Dockerfile"},
				},
			}, {
				ProjectDirectoryImageBuildStepConfiguration: &api.ProjectDirectoryImageBuildStepConfiguration{
					To: "ci-bundle0",
					ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
						ContextDir:     "manifests/olm",
						DockerfilePath: "bundle.Dockerfile",
					},
				},
			}, {
				SourceStepConfiguration: &api.SourceStepConfiguration{
					From:           "root",
					To:             "src",
					ClonerefsImage: api.ImageStreamTagReference{Namespace: "ci", Name: "clonerefs", Tag: "latest"},
					ClonerefsPath:  "/app/prow/cmd/clonerefs/app.binary.runfiles/io_k8s_test_infra/prow/cmd/clonerefs/linux_amd64_pure_stripped/app.binary",
				},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rawSteps, err := stepConfigsForBuild(testCase.input, testCase.jobSpec, testCase.readFile)
			if err != nil {
				t.Fatalf("failed to get stepConfigsForBuild: %v", err)
			}
			actual := sortStepConfig(rawSteps)
			expected := sortStepConfig(testCase.output)
			if diff := cmp.Diff(actual, expected); diff != "" {
				t.Errorf("actual differs from expected: %s", diff)
			}

		})
	}
}

func sortStepConfig(in []api.StepConfiguration) []api.StepConfiguration {
	sort.Slice(in, func(i, j int) bool {
		iMarshalled, err := json.Marshal(in[i])
		if err != nil {
			panic(fmt.Sprintf("iMarshal: %v", err))
		}
		jMarshalled, err := json.Marshal(in[j])
		if err != nil {
			panic(fmt.Sprintf("jMarshal: %v", err))
		}
		return string(iMarshalled) < string(jMarshalled)
	})
	return in
}
