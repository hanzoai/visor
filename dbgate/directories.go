package dbgate

import (
	"os"
	"path/filepath"

	"github.com/beego/beego/logs"
	"github.com/casvisor/casvisor/conf"
)

var dbgateDir string

func init() {
	_, err := os.Stat("./dbgate-docker")
	if err == nil {
		dbgateDir = "./dbgate-docker"
	} else {
		dbgateDir = conf.GetConfigString("dbgateDir")
	}
}

func dataDir() string {
	dbgateWorkspaceDir := filepath.Join(dbgateDir, ".dbgate")
	ensureDirectory(dbgateWorkspaceDir)
	return dbgateWorkspaceDir
}

func ensureDirectory(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.Mkdir(dir, 0o755)
		if err != nil {
			logs.Error("Failed to create directory:%s %v", dir, err)
		}
	}
}
