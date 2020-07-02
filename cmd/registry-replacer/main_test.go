package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/ci-tools/pkg/api"
	"github.com/openshift/ci-tools/pkg/config"
	"github.com/openshift/ci-tools/pkg/testhelper"
)

func TestReplacer(t *testing.T) {
	testCases := []struct {
		name        string
		config      *api.ReleaseBuildConfiguration
		files       map[string][]byte
		expectWrite bool
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
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fakeWriter := &fakeWriter{}
			if err := replacer(fakeGithubFileGetterFactory(tc.files), fakeWriter.Write)(tc.config, &config.Info{}); err != nil {
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

func fakeGithubFileGetterFactory(data map[string][]byte) func(string, string, string) githubFileGetter {
	return func(_, _, _ string) githubFileGetter {
		return func(path string) ([]byte, error) {
			return data[path], nil
		}
	}
}

func TestExtractAllSourceImagesFromDockerfile(t *testing.T) {
	testCases := []struct {
		name           string
		in             string
		expectedError  string
		expectedResult sets.String
	}{
		{
			name:           "simple",
			in:             "FROM capetown/center:1",
			expectedResult: sets.NewString("capetown/center:1"),
		},
		{
			name:           "multi-stage",
			in:             "from capetown/center:1 as builder\nFROM builder",
			expectedResult: sets.NewString("capetown/center:1"),
		},
		{
			name: "unrelated directives",
			in:   "RUN somestuff\n\n\n ENV var=val",
		},
		{
			name:          "defunct from",
			in:            "from\n\n",
			expectedError: `extracting words from line "from" did not yield two or four but 1 results`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualErrMsg string
			result, err := extractAllSourceImagesFromDockerfile([]byte(tc.in))
			if err != nil {
				actualErrMsg = err.Error()
			}
			if actualErrMsg != tc.expectedError {
				t.Fatalf("expected error %s, got error %s", tc.expectedError, actualErrMsg)
			}
			if err != nil {
				return
			}

			if diff := result.Difference(tc.expectedResult); len(diff) > 0 {
				t.Errorf("result differs from expected: %v", diff.List())
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pruneUnusedReplacements(tc.in, tc.allSourceImages)
			if diff := cmp.Diff(tc.in, tc.expected); diff != "" {
				t.Errorf("result differs from expected: %s", diff)
			}
		})
	}
}
