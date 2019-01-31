package secret

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/flant/werf/cmd/werf/common"
	secret_common "github.com/flant/werf/cmd/werf/helm/secret/common"
	"github.com/flant/werf/pkg/deploy/secret"
	"github.com/flant/werf/pkg/werf"
)

var CmdData struct {
	FilePath       string
	OutputFilePath string
	Values         bool
}

var CommonCmdData common.CmdData

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "extract",
		DisableFlagsInUseLine: true,
		Short: "Decrypt data",
		Long: common.GetLongCommandDescription(`Decrypt data.

Provide encrypted data onto stdin by default.

Data can be provided in a file by specifying --file-path option. Option --values should be specified in the case when secret values yaml file provided.`),
		Annotations: map[string]string{
			common.CmdEnvAnno: common.EnvsDescription(common.WerfSecretKey),
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runSecretExtract()
			if err != nil {
				return fmt.Errorf("secret extract failed: %s", err)
			}
			return nil
		},
	}

	common.SetupDir(&CommonCmdData, cmd)
	common.SetupTmpDir(&CommonCmdData, cmd)
	common.SetupHomeDir(&CommonCmdData, cmd)

	cmd.Flags().StringVarP(&CmdData.FilePath, "file-path", "", "", "Decode file data by specified path")
	cmd.Flags().StringVarP(&CmdData.OutputFilePath, "output-file-path", "", "", "Save decoded data by specified file path")
	cmd.Flags().BoolVarP(&CmdData.Values, "values", "", false, "Decode specified FILE_PATH (--file-path) as secret values file")

	return cmd
}

func runSecretExtract() error {
	if err := werf.Init(*CommonCmdData.TmpDir, *CommonCmdData.HomeDir); err != nil {
		return fmt.Errorf("initialization error: %s", err)
	}

	projectDir, err := common.GetProjectDir(&CommonCmdData)
	if err != nil {
		return fmt.Errorf("getting project dir failed: %s", err)
	}

	options := &secret_common.GenerateOptions{
		FilePath:       CmdData.FilePath,
		OutputFilePath: CmdData.OutputFilePath,
		Values:         CmdData.Values,
	}

	m, err := secret.GetManager(projectDir)
	if err != nil {
		return err
	}

	return secretExtract(m, options)
}

func secretExtract(m secret.Manager, options *secret_common.GenerateOptions) error {
	var encodedData []byte
	var data []byte
	var err error

	if options.FilePath != "" {
		encodedData, err = secret_common.ReadFileData(options.FilePath)
		if err != nil {
			return err
		}
	} else {
		encodedData, err = secret_common.ReadStdin()
		if err != nil {
			return err
		}

		if len(encodedData) == 0 {
			return nil
		}
	}

	encodedData = bytes.TrimSpace(encodedData)

	if options.FilePath != "" && options.Values {
		data, err = m.ExtractYamlData(encodedData)
		if err != nil {
			return err
		}
	} else {
		data, err = m.Extract(encodedData)
		if err != nil {
			return err
		}
	}

	if options.OutputFilePath != "" {
		if err := secret_common.SaveGeneratedData(options.OutputFilePath, data); err != nil {
			return err
		}
	} else {
		if terminal.IsTerminal(int(os.Stdout.Fd())) {
			if !bytes.HasSuffix(data, []byte("\n")) {
				data = append(data, []byte("\n")...)
			}
		}

		fmt.Printf(string(data))
	}

	return nil
}
