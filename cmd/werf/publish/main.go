package publish

import (
	"github.com/spf13/cobra"

	"github.com/flant/werf/cmd/werf/common"

	images_publish "github.com/flant/werf/cmd/werf/images/publish"
)

var CmdData images_publish.CmdDataType
var CommonCmdData common.CmdData

func NewCmd() *cobra.Command {
	return images_publish.NewCmdWithData(&CmdData, &CommonCmdData)
}
