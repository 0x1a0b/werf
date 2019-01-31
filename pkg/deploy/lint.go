package deploy

import (
	"fmt"
	"os"

	"github.com/flant/werf/pkg/config"
)

type LintOptions struct {
	Values       []string
	SecretValues []string
	Set          []string
	SetString    []string
}

func RunLint(projectDir string, werfConfig *config.WerfConfig, opts LintOptions) error {
	if debug() {
		fmt.Printf("Lint options: %#v\n", opts)
	}

	m, err := getSafeSecretManager(projectDir, opts.SecretValues)
	if err != nil {
		return fmt.Errorf("cannot get project secret: %s", err)
	}

	imagesRepo := "REPO"
	tag := "DOCKER_TAG"
	namespace := "NAMESPACE"

	images := GetImagesInfoGetters(werfConfig.Images, imagesRepo, tag, true)

	serviceValues, err := GetServiceValues(werfConfig.Meta.Project, imagesRepo, namespace, tag, nil, images, ServiceValuesOptions{ForceBranch: "GIT_BRANCH"})
	if err != nil {
		return fmt.Errorf("error creating service values: %s", err)
	}

	werfChart, err := getWerfChart(werfConfig.Meta.Project, projectDir, m, opts.Values, opts.SecretValues, opts.Set, opts.SetString, serviceValues)
	if err != nil {
		return err
	}
	if !debug() {
		// Do not remove tmp chart in debug
		defer os.RemoveAll(werfChart.ChartDir)
	}

	return werfChart.Lint()
}
