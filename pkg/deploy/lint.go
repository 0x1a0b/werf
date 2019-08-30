package deploy

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/flant/logboek"

	"github.com/flant/werf/cmd/werf/common"
	"github.com/flant/werf/pkg/config"
	"github.com/flant/werf/pkg/deploy/helm"
	"github.com/flant/werf/pkg/tag_strategy"
)

type LintOptions struct {
	Values          []string
	SecretValues    []string
	Set             []string
	SetString       []string
	Env             string
	IgnoreSecretKey bool
}

func RunLint(projectDir string, werfConfig *config.WerfConfig, opts LintOptions) error {
	if debug() {
		fmt.Fprintf(logboek.GetOutStream(), "Lint options: %#v\n", opts)
	}

	m, err := GetSafeSecretManager(projectDir, opts.SecretValues, opts.IgnoreSecretKey)
	if err != nil {
		return err
	}

	imagesRepoManager, err := common.GetImagesRepoManager("REPO", common.MultirepImagesRepoMode)
	if err != nil {
		return err
	}

	tag := "GIT_BRANCH"
	tagStrategy := tag_strategy.GitBranch
	namespace := "NAMESPACE"

	images := GetImagesInfoGetters(werfConfig.StapelImages, werfConfig.ImagesFromDockerfile, imagesRepoManager, tag, true)

	serviceValues, err := GetServiceValues(werfConfig.Meta.Project, imagesRepoManager, namespace, tag, tagStrategy, images, ServiceValuesOptions{Env: opts.Env})
	if err != nil {
		return fmt.Errorf("error creating service values: %s", err)
	}

	werfChart, err := PrepareWerfChart(GetTmpWerfChartPath(werfConfig.Meta.Project), werfConfig.Meta.Project, projectDir, opts.Env, m, opts.SecretValues, serviceValues)
	if err != nil {
		return err
	}
	defer ReleaseTmpWerfChart(werfChart.ChartDir)

	if err := helm.Lint(
		os.Stdout,
		werfChart.ChartDir,
		namespace,
		append(werfChart.Values, opts.Values...),
		append(werfChart.Set, opts.Set...),
		append(werfChart.SetString, opts.SetString...),
		helm.LintOptions{Strict: true},
	); err != nil {
		replaceOld := fmt.Sprintf("%s/", werfChart.Name)
		replaceNew := fmt.Sprintf("%s/", ".helm")
		errMsg := strings.Replace(err.Error(), replaceOld, replaceNew, -1)
		return errors.New(errMsg)
	}

	return nil
}
