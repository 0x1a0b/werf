package common

import (
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/util/file"

	"github.com/flant/kubedog/pkg/kube"
	"github.com/flant/werf/pkg/config"
	"github.com/flant/werf/pkg/logger"
	"github.com/flant/werf/pkg/werf"
)

type CmdData struct {
	Dir     *string
	TmpDir  *string
	HomeDir *string
	SSHKeys *[]string

	Tag        *[]string
	TagBranch  *bool
	TagBuildID *bool
	TagCI      *bool
	TagCommit  *bool

	Environment *string
	Release     *string
	Namespace   *string
	KubeContext *string

	StagesRepo *string
	ImagesRepo *string
}

func GetLongCommandDescription(text string) string {
	return logger.FitTextWithIndentWithWidthMaxLimit(text, 0, 100)
}

func SetupDir(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.Dir = new(string)
	cmd.Flags().StringVarP(cmdData.Dir, "dir", "", "", "Change to the specified directory to find werf.yaml config")
}

func SetupTmpDir(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.TmpDir = new(string)
	cmd.Flags().StringVarP(cmdData.TmpDir, "tmp-dir", "", "", "Use specified dir to store tmp files and dirs (use system tmp dir by default)")
}

func SetupHomeDir(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.HomeDir = new(string)
	cmd.Flags().StringVarP(cmdData.HomeDir, "home-dir", "", "", "Use specified dir to store werf cache files and dirs (use ~/.werf by default)")
}

func SetupSSHKey(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.SSHKeys = new([]string)
	cmd.Flags().StringArrayVarP(cmdData.SSHKeys, "ssh-key", "", []string{}, "Enable only specified ssh keys (use system ssh-agent by default)")
}

func SetupTag(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.Tag = new([]string)
	cmdData.TagBranch = new(bool)
	cmdData.TagBuildID = new(bool)
	cmdData.TagCI = new(bool)
	cmdData.TagCommit = new(bool)

	cmd.Flags().StringArrayVarP(cmdData.Tag, "tag", "", []string{}, "Add tag (can be used one or more times)")
	cmd.Flags().BoolVarP(cmdData.TagBranch, "tag-branch", "", false, "Tag by git branch")
	cmd.Flags().BoolVarP(cmdData.TagBuildID, "tag-build-id", "", false, "Tag by CI build id")
	cmd.Flags().BoolVarP(cmdData.TagCI, "tag-ci", "", false, "Tag by CI branch and tag")
	cmd.Flags().BoolVarP(cmdData.TagCommit, "tag-commit", "", false, "Tag by git commit")
}

func SetupEnvironment(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.Environment = new(string)
	cmd.Flags().StringVarP(cmdData.Environment, "env", "", "", "Use specified environment (use CI_ENVIRONMENT_SLUG by default). Environment is a required parameter and should be specified with option or CI_ENVIRONMENT_SLUG variable.")
}

func SetupRelease(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.Release = new(string)
	cmd.Flags().StringVarP(cmdData.Release, "release", "", "", "Use specified Helm release name (use %project-%environment template by default)")
}

func SetupNamespace(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.Namespace = new(string)
	cmd.Flags().StringVarP(cmdData.Namespace, "namespace", "", "", "Use specified Kubernetes namespace (use %project-%environment template by default)")
}

func SetupKubeContext(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.KubeContext = new(string)
	cmd.Flags().StringVarP(cmdData.KubeContext, "kube-context", "", "", "Kubernetes config context")
}

func SetupStagesRepo(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.StagesRepo = new(string)
	cmd.Flags().StringVarP(cmdData.StagesRepo, "stages", "s", "", "Docker Repo to store stages or :local for non-distributed build (only :local is supported for now)")
}

func SetupImagesRepo(cmdData *CmdData, cmd *cobra.Command) {
	cmdData.ImagesRepo = new(string)
	cmd.Flags().StringVarP(cmdData.ImagesRepo, "images", "i", "", "Docker Repo to store images")
}

func GetStagesRepo(cmdData *CmdData) (string, error) {
	if *cmdData.StagesRepo == "" {
		return "", fmt.Errorf("--stages :local param required")
	} else if *cmdData.StagesRepo != ":local" {
		return "", fmt.Errorf("only --stages :local is supported for now, got '%s'", *cmdData.StagesRepo)
	}
	return *cmdData.StagesRepo, nil
}

func GetImagesRepo(projectName string, cmdData *CmdData) (string, error) {
	if *cmdData.ImagesRepo == "" {
		return "", fmt.Errorf("--images REPO param required")
	}
	return GetOptionalImagesRepo(projectName, *cmdData.ImagesRepo), nil
}

func GetOptionalImagesRepo(projectName, repoOption string) string {
	if repoOption == ":minikube" {
		return fmt.Sprintf("werf-registry.kube-system.svc.cluster.local:5000/%s", projectName)
	} else if repoOption != "" {
		return repoOption
	}

	ciRegistryImage := os.Getenv("CI_REGISTRY_IMAGE")
	if ciRegistryImage != "" {
		return ciRegistryImage
	}

	return ""
}

func GetWerfConfig(projectDir string) (*config.WerfConfig, error) {
	for _, werfConfigName := range []string{"werf.yml", "werf.yaml"} {
		werfConfigPath := path.Join(projectDir, werfConfigName)
		if exist, err := file.FileExists(werfConfigPath); err != nil {
			return nil, err
		} else if exist {
			return config.ParseWerfConfig(werfConfigPath)
		}
	}

	return nil, errors.New("werf.yaml not found")
}

func GetProjectDir(cmdData *CmdData) (string, error) {
	if *cmdData.Dir != "" {
		return *cmdData.Dir, nil
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return currentDir, nil
}

func GetProjectBuildDir(projectName string) (string, error) {
	projectBuildDir := path.Join(werf.GetHomeDir(), "builds", projectName)

	if err := os.MkdirAll(projectBuildDir, os.ModePerm); err != nil {
		return "", err
	}

	return projectBuildDir, nil
}

func GetNamespace(namespaceOption string) string {
	if namespaceOption == "" {
		return kube.DefaultNamespace
	}
	return namespaceOption
}

func LogRunningTime(f func() error) error {
	t := time.Now()
	err := f()

	logger.LogService(fmt.Sprintf("Running time %0.2f seconds", time.Now().Sub(t).Seconds()))

	return err
}

func LogVersion() {
	logger.LogInfoF("Version: %s\n", werf.Version)
}

func LogProjectDir(dir string) {
	if os.Getenv("CI") != "" {
		logger.LogInfoF("Using project dir: %s\n", dir)
	}
}
