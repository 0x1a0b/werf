package rm

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/flant/shluz"
	"github.com/spf13/cobra"

	"github.com/flant/werf/cmd/werf/common"
	"github.com/flant/werf/pkg/container_runtime"
	"github.com/flant/werf/pkg/docker"
	"github.com/flant/werf/pkg/tmp_manager"
	"github.com/flant/werf/pkg/werf"
)

var commonCmdData common.CmdData

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "rm",
		DisableFlagsInUseLine: true,
		Short:                 "Remove image record from the list of managed images which will be preserved during cleanup procedure",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := common.ProcessLogOptions(&commonCmdData); err != nil {
				common.PrintHelp(cmd)
				return err
			}

			if err := common.ValidateMinimumNArgs(1, args, cmd); err != nil {
				return err
			}
			return run(args)
		},
	}

	common.SetupProjectName(&commonCmdData, cmd)
	common.SetupDir(&commonCmdData, cmd)
	common.SetupTmpDir(&commonCmdData, cmd)
	common.SetupHomeDir(&commonCmdData, cmd)
	common.SetupSSHKey(&commonCmdData, cmd)

	common.SetupStagesStorageOptions(&commonCmdData, cmd)

	common.SetupSynchronization(&commonCmdData, cmd)
	common.SetupDockerConfig(&commonCmdData, cmd, "Command needs granted permissions to read and write images to the specified stages storage")
	common.SetupInsecureRegistry(&commonCmdData, cmd)
	common.SetupSkipTlsVerifyRegistry(&commonCmdData, cmd)

	common.SetupLogOptions(&commonCmdData, cmd)
	common.SetupLogProjectDir(&commonCmdData, cmd)

	return cmd
}

func run(imageNames []string) error {
	if err := werf.Init(*commonCmdData.TmpDir, *commonCmdData.HomeDir); err != nil {
		return fmt.Errorf("initialization error: %s", err)
	}

	if err := shluz.Init(filepath.Join(werf.GetServiceDir(), "locks")); err != nil {
		return err
	}

	if err := common.DockerRegistryInit(&commonCmdData); err != nil {
		return err
	}

	if err := docker.Init(*commonCmdData.DockerConfig, *commonCmdData.LogVerbose, *commonCmdData.LogDebug); err != nil {
		return err
	}

	projectDir, err := common.GetProjectDir(&commonCmdData)
	if err != nil {
		return fmt.Errorf("getting project dir failed: %s", err)
	}

	projectTmpDir, err := tmp_manager.CreateProjectDir()
	if err != nil {
		return fmt.Errorf("getting project tmp dir failed: %s", err)
	}
	defer tmp_manager.ReleaseProjectDir(projectTmpDir)

	werfConfig, err := common.GetOptionalWerfConfig(projectDir, false)
	if err != nil {
		return fmt.Errorf("unable to load werf config: %s", err)
	}

	var projectName string
	if werfConfig != nil {
		projectName = werfConfig.Meta.Project
	} else if *commonCmdData.ProjectName != "" {
		projectName = *commonCmdData.ProjectName
	} else {
		return fmt.Errorf("run command in the project directory with werf.yaml or specify --project-name=PROJECT_NAME param")
	}

	containerRuntime := &container_runtime.LocalDockerServerRuntime{} // TODO

	stagesStorage, err := common.GetStagesStorage(containerRuntime, &commonCmdData)
	if err != nil {
		return err
	}

	if _, err = common.GetSynchronization(&commonCmdData); err != nil {
		return err
	}

	errs := []error{}
	for _, imageName := range imageNames {
		if err := stagesStorage.RmManagedImage(projectName, common.GetManagedImageName(imageName)); err != nil {
			errs = append(errs, fmt.Errorf("unable to remove known config image name %q of project %q: %s", imageName, projectName, err))
		}
	}

	if len(errs) > 0 {
		errMsgs := []string{}
		for _, err := range errs {
			errMsgs = append(errMsgs, err.Error())
		}
		return fmt.Errorf("%s", strings.Join(errMsgs, "; "))
	}

	return nil
}