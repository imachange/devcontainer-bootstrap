// devcontainer-bootstrap は、dev container 起動前にホスト側で実行される初期化ツールです。
package main

import (
	"os"

	"github.com/imachange/container-bootstrap/internal/cli"
)

func main() {
	os.Exit(cli.NewApp().Run(os.Args[1:]))
}
