package steps

import (
	"context"
	"fmt"
	"strings"

	coreapi "k8s.io/api/core/v1"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	buildapi "github.com/openshift/api/build/v1"

	"github.com/openshift/ci-tools/pkg/api"
	"github.com/openshift/ci-tools/pkg/results"
	"github.com/openshift/ci-tools/pkg/steps/utils"
)

type indexGeneratorStep struct {
	config             api.IndexGeneratorStepConfiguration
	releaseBuildConfig *api.ReleaseBuildConfiguration
	resources          api.ResourceConfiguration
	buildClient        BuildClient
	client             ctrlruntimeclient.Client
	jobSpec            *api.JobSpec
	artifactDir        string
	pullSecret         *coreapi.Secret
}

const IndexDataDirectory = "/index-data"
const IndexDockerfileName = "index.Dockerfile"

func (s *indexGeneratorStep) Inputs() (api.InputDefinition, error) {
	return nil, nil
}

func (*indexGeneratorStep) Validate() error { return nil }

func (s *indexGeneratorStep) Run(ctx context.Context) error {
	return results.ForReason("building_index_generator").ForError(s.run(ctx))
}

func (s *indexGeneratorStep) run(ctx context.Context) error {
	source := fmt.Sprintf("%s:%s", api.PipelineImageStream, api.PipelineImageStreamTagReferenceSource)
	workingDir, err := getWorkingDir(s.client, source, s.jobSpec.Namespace())
	if err != nil {
		return fmt.Errorf("failed to get workingDir: %w", err)
	}
	dockerfile, err := s.indexGenDockerfile()
	if err != nil {
		return err
	}
	build := buildFromSource(
		s.jobSpec, api.PipelineImageStreamTagReferenceSource, s.config.To,
		buildapi.BuildSource{
			Type:       buildapi.BuildSourceDockerfile,
			Dockerfile: &dockerfile,
			Images: []buildapi.ImageSource{
				{
					From: coreapi.ObjectReference{
						Kind: "ImageStreamTag",
						Name: source,
					},
					Paths: []buildapi.ImageSourcePath{{
						SourcePath:     fmt.Sprintf("%s/.", workingDir),
						DestinationDir: ".",
					}},
				},
			},
			Secrets: []buildapi.SecretBuildSource{{
				Secret: coreapi.LocalObjectReference{Name: s.pullSecret.Name},
			}},
		},
		"",
		s.resources,
		s.pullSecret,
	)
	err = handleBuild(ctx, s.buildClient, build, s.artifactDir)
	if err != nil && strings.Contains(err.Error(), "error checking provided apis") {
		return results.ForReason("generating_index").WithError(err).Errorf("failed to generate operator index due to invalid bundle info: %v", err)
	}
	return err
}

func (s *indexGeneratorStep) indexGenDockerfile() (string, error) {
	var dockerCommands []string
	dockerCommands = append(dockerCommands, "FROM quay.io/operator-framework/upstream-opm-builder AS builder")
	// pull secret is needed for opm command
	dockerCommands = append(dockerCommands, "COPY .dockerconfigjson .")
	dockerCommands = append(dockerCommands, "RUN mkdir $HOME/.docker && mv .dockerconfigjson $HOME/.docker/config.json")
	var bundles []string
	for _, bundleName := range s.config.OperatorIndex {
		fullSpec, err := utils.ImageDigestFor(s.client, s.jobSpec.Namespace, api.PipelineImageStream, bundleName)()
		if err != nil {
			return "", fmt.Errorf("failed to get image digest for bundle `%s`: %w", bundleName, err)
		}
		bundles = append(bundles, fullSpec)
	}
	dockerCommands = append(dockerCommands, fmt.Sprintf(`RUN ["opm", "index", "add", "--mode", "semver", "--bundles", "%s", "--out-dockerfile", "%s", "--generate"]`, strings.Join(bundles, ","), IndexDockerfileName))
	dockerCommands = append(dockerCommands, fmt.Sprintf("FROM %s:%s", api.PipelineImageStream, api.PipelineImageStreamTagReferenceSource))
	dockerCommands = append(dockerCommands, fmt.Sprintf("WORKDIR %s", IndexDataDirectory))
	dockerCommands = append(dockerCommands, fmt.Sprintf("COPY --from=builder %s %s", IndexDockerfileName, IndexDockerfileName))
	dockerCommands = append(dockerCommands, "COPY --from=builder /database/ database")
	return strings.Join(dockerCommands, "\n"), nil
}

func (s *indexGeneratorStep) Requires() []api.StepLink {
	var links []api.StepLink
	for _, bundle := range s.config.OperatorIndex {
		imageStream, name, _ := s.releaseBuildConfig.DependencyParts(api.StepDependency{Name: bundle})
		links = append(links, api.LinkForImage(imageStream, name))
	}
	return links
}

func (s *indexGeneratorStep) Creates() []api.StepLink {
	return []api.StepLink{api.InternalImageLink(s.config.To)}
}

func (s *indexGeneratorStep) Provides() api.ParameterMap {
	return api.ParameterMap{}
}

func (s *indexGeneratorStep) Name() string { return string(s.config.To) }

func (s *indexGeneratorStep) Description() string {
	return fmt.Sprintf("Build image %s from the repository", s.config.To)
}

func IndexGeneratorStep(config api.IndexGeneratorStepConfiguration, releaseBuildConfig *api.ReleaseBuildConfiguration, resources api.ResourceConfiguration, buildClient BuildClient, client ctrlruntimeclient.Client, artifactDir string, jobSpec *api.JobSpec, pullSecret *coreapi.Secret) api.Step {
	return &indexGeneratorStep{
		config:             config,
		releaseBuildConfig: releaseBuildConfig,
		resources:          resources,
		buildClient:        buildClient,
		client:             client,
		artifactDir:        artifactDir,
		jobSpec:            jobSpec,
		pullSecret:         pullSecret,
	}
}
