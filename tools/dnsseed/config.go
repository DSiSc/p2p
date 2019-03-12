package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/DSiSc/craft/log"
	"github.com/spf13/viper"
	"math"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// config file prefix
	ConfigPrefix = "justitia"
	// General setting
	ListenAddress    = "general.listenAddr"
	AddrBookFilePath = "general.addrBookFilePath"
	MaxConnOutBound  = "general.maxConnOutBound"
	MaxConnInBound   = "general.maxConnInBound"

	// Log Setting
	LogTimeFieldFormat = "logging.timeFieldFormat"
	ConsoleLogAppender = "logging.console"
	LogConsoleEnabled  = "logging.console.enabled"
	LogConsoleLevel    = "logging.console.level"
	LogConsoleFormat   = "logging.console.format"
	LogConsoleCaller   = "logging.console.caller"
	LogConsoleHostname = "logging.console.hostname"
	FileLogAppender    = "logging.file"
	LogFileEnabled     = "logging.file.enabled"
	LogFilePath        = "logging.file.path"
	LogFileLevel       = "logging.file.level"
	LogFileFormat      = "logging.file.format"
	LogFileCaller      = "logging.file.caller"
	LogFileHostname    = "logging.file.hostname"
)

type SysConfig struct {
	LogLevel log.Level
	LogPath  string
	LogStyle string
}

type NodeConfig struct {
	AddrBookFilePath string     // address book file path
	ListenAddress    string     // server listen address
	MaxConnOutBound  int        // max connection out bound
	MaxConnInBound   int        // max connection in bound
	Logger           log.Config // log setting
}

type Config struct {
	filePath string
	maps     map[string]interface{}
}

func LoadConfig() (config *viper.Viper) {
	config = viper.New()
	// for environment variables
	config.SetEnvPrefix(ConfigPrefix)
	config.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	config.SetEnvKeyReplacer(replacer)

	config.SetConfigName("justitia")
	homePath, _ := Home()
	config.AddConfigPath(fmt.Sprintf("%s/.justitia", homePath))
	// Path to look for the config file in based on GOPATH
	goPath := os.Getenv("GOPATH")
	for _, p := range filepath.SplitList(goPath) {
		config.AddConfigPath(filepath.Join(p, "src/github.com/DSiSc/p2p/tools/dnsseed"))
	}

	err := config.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("error reading plugin config: %s", err))
	}
	return
}

func NewNodeConfig() NodeConfig {
	config := LoadConfig()
	listenAddr := config.GetString(ListenAddress)
	addrBookFilePath := config.GetString(AddrBookFilePath)
	maxConnOutBound := config.GetInt(MaxConnOutBound)
	maxConnInBound := config.GetInt(MaxConnInBound)
	logConf := GetLogSetting(config)
	return NodeConfig{
		ListenAddress:    listenAddr,
		AddrBookFilePath: addrBookFilePath,
		MaxConnOutBound:  maxConnOutBound,
		MaxConnInBound:   maxConnInBound,
		Logger:           logConf,
	}
}

func GetLogSetting(conf *viper.Viper) log.Config {
	logTimestampFormat := conf.GetString(LogTimeFieldFormat)
	logConsoleEnabled := conf.GetBool(LogConsoleEnabled)
	logConsoleLevel := conf.GetInt(LogConsoleLevel)
	logConsoleFormat := conf.GetString(LogConsoleFormat)
	logConsoleCaller := conf.GetBool(LogConsoleCaller)
	logConsoleHostname := conf.GetBool(LogConsoleHostname)
	logFileEnabled := conf.GetBool(LogFileEnabled)
	logFilePath := conf.GetString(LogFilePath)
	logFileLevel := conf.GetInt(LogFileLevel)
	logFileFormat := conf.GetString(LogFileFormat)
	logFileCaller := conf.GetBool(LogFileCaller)
	logFileHostname := conf.GetBool(LogFileHostname)

	consoleAppender := &log.Appender{
		Enabled:      logConsoleEnabled,
		LogLevel:     log.Level(logConsoleLevel),
		LogType:      log.ConsoleLog,
		LogPath:      log.ConsoleStdout,
		Output:       os.Stdout,
		Format:       strings.ToUpper(logConsoleFormat),
		ShowCaller:   logConsoleCaller,
		ShowHostname: logConsoleHostname,
	}
	//tools.EnsureFolderExist(logFilePath[0:strings.LastIndex(logFilePath, "/")])
	//logfile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	//if err != nil {
	//	panic(err)
	//}
	fileAppender := &log.Appender{
		Enabled:      logFileEnabled,
		LogLevel:     log.Level(logFileLevel),
		LogType:      log.FileLog,
		LogPath:      logFilePath,
		Output:       nil,
		Format:       strings.ToUpper(logFileFormat),
		ShowCaller:   logFileCaller,
		ShowHostname: logFileHostname,
	}

	globalLogConfig := log.Config{
		Enabled:         logConsoleEnabled || logFileEnabled,
		Provider:        log.GetGlobalConfig().Provider,
		GlobalLogLevel:  log.Level(uint8(math.Max(float64(logConsoleLevel), float64(logFileLevel)))),
		TimeFieldFormat: logTimestampFormat,
		Appenders:       map[string]*log.Appender{ConsoleLogAppender: consoleAppender, FileLogAppender: fileAppender},
		OutputFlags:     log.GetOutputFlags(),
	}
	return globalLogConfig
}

// Home returns the home directory for the executing user.
//
// This uses an OS-specific method for discovering the home directory.
// An error is returned if a home directory cannot be detected.
func Home() (string, error) {
	user, err := user.Current()
	if nil == err {
		return user.HomeDir, nil
	}

	if "windows" == runtime.GOOS {
		return homeWindows()
	}

	// Unix-like system, so just assume Unix
	return homeUnix()
}

func homeUnix() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	// If that fails, try the shell
	var stdout bytes.Buffer
	cmd := exec.Command("sh", "-c", "eval echo ~$USER")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		log.Error("sh -c eval echo ~$USER error.")
		return "", err
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		log.Error("blank output when reading home directory")
		return "", errors.New("blank output when reading home directory")
	}

	return result, nil
}

func homeWindows() (string, error) {
	drive := os.Getenv("HOMEDRIVE")
	path := os.Getenv("HOMEPATH")
	home := drive + path
	if drive == "" || path == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		log.Error("Get home path error.")
		return "", errors.New("HOMEDRIVE, HOMEPATH, and USERPROFILE are blank")
	}

	return home, nil
}

func EnsureFolderExist(folderPath string) {
	_, err := os.Stat(folderPath)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(folderPath, 0755)
			if err != nil {
				log.Error("Can not create folder %s: %v", folderPath, err)
			}
		} else {
			log.Error("Can not create folder %s: %v", folderPath, err)
		}
	}
}
