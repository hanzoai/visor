package dbgate

import (
	"os"

	"github.com/beego/beego/logs"
)

const (
	dbgateDir = "../dbgate/packages/api/data"
)

func dataDir() string {
	ensureDirectory(dbgateDir)
	return dbgateDir
}

func ensureDirectory(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(dir, os.ModePerm)
		if err != nil {
			logs.Error("Failed to create directory:%s", dir)
		}
	}
}
