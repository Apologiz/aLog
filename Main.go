// Author:   Nikita Koryabkin
// Email:    Nikita@Koryabk.in
// Telegram: https://t.me/Apologiz

package alog

import (
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	loggerInfo uint = iota
	loggerWrn
	loggerErr
)

const (
	keyInfo = "ALOG_LOGGER_INFO"
	keyWrn  = "ALOG_LOGGER_WARNING"
	keyErr  = "ALOG_LOGGER_ERROR"
)

const (
	fileOptions    = os.O_CREATE | os.O_APPEND | os.O_WRONLY
	filePermission = 0755
)

var self *aLog
var instance sync.Once
var configurator sync.Once

func getEnv(key string) []byte {
	configurator.Do(func() {
		if err := godotenv.Load(); err != nil {
			log.Fatalln(err)
		}
	})
	return []byte(os.Getenv(key))
}

func getEnvStr(key string) string {
	return string(getEnv(key))
}

type Logger struct {
	class    uint
	filePath string
	file     *os.File
	channel  chan string
}

func (l *Logger) addLogger(logType uint, filePath string) *Logger {
	if addDirectory(filePath) {
		if file, err := openFile(filePath); err == nil {
			l.file = file
		} else {
			fatalError(err)
		}
	}
	return l
}

func (l *Logger) conveyor() {
	defer func() {
		fatalError(l.file.Close())
	}()
	for {
		select {
		case msg := <-l.channel:
			_, err := l.file.WriteString(msg)
			fatalError(err)
		}
	}
}

type aLog struct {
	Loggers []Logger
}

// Writer interface for informational messages
func (l *Logger) Write(p []byte) (n int, err error) {
	msg := string(p)
	l.channel <- msg
	return utf8.RuneCountInString(msg), nil
}

// Returns the info channel to write
func GetInfoLogger() *Logger {
	return &get().Loggers[loggerInfo]
}

// Returns the warning channel to write
func GetWarningLogger() *Logger {
	return &get().Loggers[loggerWrn]
}

// Returns the error channel to write
func GetErrorLogger() *Logger {
	return &get().Loggers[loggerErr]
}

// Method for recording informational messages
func Info(msg string) {
	GetInfoLogger().channel <- prepareLog(msg)
}

// Method of recording formatted informational messages
func Infof(format string, a ...interface{}) {
	GetInfoLogger().channel <- prepareLog(fmt.Sprintf(format, a...))
}

// Method for recording warning messages
func Warning(msg string) {
	GetWarningLogger().channel <- prepareLog(msg)
}

// Method for recording errors with stack
func Error(err error) {
	if err != nil {
		GetErrorLogger().channel <- fmt.Sprintf("%s\n%s\n---\n\n", prepareLog(err.Error()), string(debug.Stack()))
	}
}

func (a *aLog) getLoggers() []Logger {
	a.Loggers = []Logger{
		{
			class:    loggerInfo,
			filePath: getEnvStr(keyInfo),
			channel:  make(chan string, 100),
		},
		{
			class:    loggerWrn,
			filePath: getEnvStr(keyWrn),
			channel:  make(chan string, 100),
		},
		{
			class:    loggerErr,
			filePath: getEnvStr(keyErr),
			channel:  make(chan string, 100),
		},
	}
	return a.Loggers
}

func (a *aLog) create() {
	loggers := a.getLoggers()
	for idx := range loggers {
		loggers[idx].addLogger(loggers[idx].class, loggers[idx].filePath)
		go loggers[idx].conveyor()
	}
}

func get() *aLog {
	instance.Do(func() {
		self = new(aLog)
		self.create()
	})
	return self
}

func prepareLog(msg string) string {
	_, fileName, fileLine, ok := runtime.Caller(2)
	if ok {
		return fmt.Sprintf(
			"%s;%s:%d;%s\n",
			time.Now().Format(time.RFC3339),
			fileName,
			fileLine,
			msg,
		)
	}
	return fmt.Sprintf(
		"%s;;%s\n",
		time.Now().Format(time.RFC3339),
		msg,
	)
}

func fatalError(err error) {
	if err != nil {
		log.Panicln(err)
	}
}

func openFile(filePath string) (*os.File, error) {
	return os.OpenFile(filePath, fileOptions, filePermission)
}

func createDirectoryIfNotExist(dirPath string) error {
	_, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return os.MkdirAll(dirPath, filePermission)
	}
	return err
}

func addDirectory(filePath string) bool {
	path := strings.Split(filePath, "/")
	err := createDirectoryIfNotExist(strings.Join(path[:len(path)-1], "/"))
	return err == nil
}