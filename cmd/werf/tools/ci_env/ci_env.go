package ci_env

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/flant/werf/pkg/docker_registry"
)

var CmdData struct {
	TaggingStrategy string
}

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "ci-env CI_SYSTEM",
		DisableFlagsInUseLine: true,
		Short:                 "Generate werf environment variables for specified CI system",
		Long: `Generate werf environment variables for specified CI system.

Currently supported only GitLab CI`,
		Example: `  # Load generated werf environment variables on gitlab job runner
  source <(werf ci-env gitlab --tagging-strategy tag-or-branch)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runCIEnv(args); err != nil {
				cmd.Help()
				fmt.Println()

				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&CmdData.TaggingStrategy, "tagging-strategy", "", "", "tag-or-branch: generate auto '--tag-git-branch' or '--tag-git-tag' tag by specified CI_SYSTEM environment variables")

	return cmd
}

func runCIEnv(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("accepts 1 position argument, received %d", len(args))
	}

	switch CmdData.TaggingStrategy {
	case "tag-or-branch":
	default:
		return fmt.Errorf("accepts tagging-strategy '%s' not supported", CmdData.TaggingStrategy)
	}

	ciSystem := args[0]

	switch ciSystem {
	case "gitlab":
		return generateGitlabEnvs()
	default:
		return fmt.Errorf("accepts ci system '%s' not supported", ciSystem)
	}
}

func generateGitlabEnvs() error {
	imagesRegistry := os.Getenv("CI_REGISTRY_IMAGE")
	var imagesUsername, imagesPassword string
	if imagesRegistry != "" {
		isGRC, err := docker_registry.IsGCR(imagesRegistry)
		if err != nil {
			return err
		}

		if isGRC && os.Getenv("CI_JOB_TOKEN") != "" {
			imagesUsername = "ci-job-token"
			imagesUsername = os.Getenv("CI_JOB_TOKEN")
		}
	}

	var ciGitTag, ciGitBranch string

	if os.Getenv("CI_BUILD_TAG") != "" {
		ciGitTag = os.Getenv("CI_BUILD_TAG")
	} else if os.Getenv("CI_COMMIT_TAG") != "" {
		ciGitTag = os.Getenv("CI_COMMIT_TAG")
	}

	if os.Getenv("CI_BUILD_REF_NAME") != "" {
		ciGitBranch = os.Getenv("CI_BUILD_REF_NAME")
	} else if os.Getenv("CI_COMMIT_REF_NAME") != "" {
		ciGitBranch = os.Getenv("CI_COMMIT_REF_NAME")
	}

	if ciGitTag == "" && ciGitBranch == "" {
		return fmt.Errorf("enviroment variables for '%s' strategy not detected", CmdData.TaggingStrategy)
	}

	fmt.Println("### IMAGES REGISTRY\n")

	printExport("export WERF_IMAGES_REGISTRY=\"%s\"\n", imagesRegistry)
	printExport("export WERF_IMAGES_USERNAME=\"%s\"\n", imagesUsername)
	printExport("export WERF_IMAGES_PASSWORD=\"%s\"\n", imagesPassword)

	fmt.Println("\n### TAGGING\n")
	printExport("export WERF_AUTOTAG_GIT_TAG=\"%s\"\n", ciGitTag)
	printExport("export WERF_AUTOTAG_GIT_BRANCH=\"%s\"\n", ciGitBranch)

	fmt.Println("\n### DEPLOY\n")
	printExport("export WERF_DEPLOY_ENVIRONMENT=\"%s\"\n", os.Getenv("CI_ENVIRONMENT_SLUG"))

	fmt.Println("\n### OTHER\n")
	printExport("export WERF_LOG_FORCE_COLOR=\"%s\"\n", "1")
	printExport("export WERF_LOG_PWD=\"%s\"\n", "1")
	printExport("export WERF_ENABLE_PROCESS_EXTERMINATOR=\"%s\"\n", "1")

	return nil
}

func printExport(format, value string) {
	if value == "" {
		format = fmt.Sprintf("# %s", format)
	}

	fmt.Printf(format, value)
}
