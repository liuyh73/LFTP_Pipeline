package log

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var Logger *log.Logger
var ExecPath string

func init() {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	index := strings.LastIndex(path, string(os.PathSeparator))
	// the current path will be $GOPATH/bin
	// so I here truncate the bin and add src to get $GOPATH/src
	ExecPath = path[:index-3] + "src/"
	ExecPath = strings.Replace(ExecPath, "\\", "/", -1)

	var logHandler *os.File
	logHandler, err := os.OpenFile(ExecPath+"github.com/liuyh73/LFTP_Pipeline/server/log/logFile.txt", os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	Logger = log.New(logHandler, "", log.LstdFlags)
}
