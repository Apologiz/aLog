////////////////////////////////////////////////////////////////////////////////
// Author:   Nikita Koryabkin
// Email:    Nikita@Koryabk.in
// Telegram: https://t.me/Apologiz
////////////////////////////////////////////////////////////////////////////////

package alog

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/mylockerteam/mailSender"
	"github.com/spf13/afero"
	"gopkg.in/gomail.v2"
)

const (
	messageFormatDefault      = "%s;%s\n"
	messageFormatErrorDebug   = "%s\n%s\n---\n\n"
	messageFormatWithFileLine = "%s;%s:%d;%s\n"
)

// Logger types
const (
	LoggerInfo uint = iota
	LoggerWrn
	LoggerErr
)

const (
	fileOptions    = os.O_CREATE | os.O_APPEND | os.O_WRONLY
	filePermission = 0755
)

var (
	errCanNotCreateDirectory = errors.New("can't create directory")
)

var fs = afero.NewOsFs()

//Logged interface for loggers
type Logged interface {
	Info(msg string) *Log
	Infof(format string, p ...interface{}) *Log
	Warning(msg string) *Log
	Error(err error) *Log
	ErrorDebug(err error) *Log
	GetLoggerInterfaceByType(loggerType uint) io.Writer
}

// Logger logger structure which includes a channel and a slice strategies
type Logger struct {
	io.Writer
	Channel    chan string
	Strategies []io.Writer
}

// LoggerMap mapping for type:logger
type LoggerMap map[uint]*Logger

// Config contains settings and registered loggers
type Config struct {
	Loggers        LoggerMap
	TimeFormat     string
	IgnoreFileLine bool
}

// Log logger himself
type Log struct {
	_      Logged
	config *Config
}

// DefaultStrategy logging strategy in the console
type DefaultStrategy struct {
	io.Writer
}

// FileStrategy logging strategy in the file
type FileStrategy struct {
	file afero.File
	io.Writer
}

// EmailStrategy logging strategy in the email
// You can use it for errors and other types of messages
type EmailStrategy struct {
	sender   mailSender.AsyncSender
	Message  *gomail.Message
	Template *template.Template
	io.Writer
}

var loggerName = map[uint]string{
	LoggerInfo: "Info",
	LoggerWrn:  "Warning",
	LoggerErr:  "Error",
}

func (l *Logger) addStrategy(strategy io.Writer) {
	l.Strategies = append(l.Strategies, strategy)
}

// LoggerName returns a name for the logger.
// It returns the empty string if the code is unknown.
func LoggerName(code uint) string {
	return loggerName[code]
}

// Writer interface for informational messages
func (l *Logger) Write(p []byte) (n int, err error) {
	if l == nil || isClosedCh(l.Channel) {
		return 0, errors.New("the channel was closed for recording")
	}
	l.Channel <- string(p)
	return len(p), nil
}

func isClosedCh(ch <-chan string) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// GetDefaultStrategy console write strategy
func GetDefaultStrategy() io.Writer {
	return &DefaultStrategy{}
}

func (s *DefaultStrategy) Write(p []byte) (n int, err error) {
	log.Println(string(p))
	return len(p), nil
}

// GetFileStrategy file write strategy
func GetFileStrategy(filePath string) io.Writer {
	if addDirectory(filePath) == nil {
		file, err := openFile(filePath)
		if err == nil {
			return &FileStrategy{
				file: file,
			}
		}
	}
	return &FileStrategy{}
}

func (s *FileStrategy) Write(p []byte) (n int, err error) {
	if s.file != nil {
		return s.file.Write(p)
	}
	return 0, errors.New("file not defined")
}

//GetEmailStrategy waiting for a parameter ess in format host:port;username;password
func GetEmailStrategy(sender mailSender.AsyncSender, msg *gomail.Message, tpl *template.Template) io.Writer {
	return &EmailStrategy{
		sender:   sender,
		Message:  msg,
		Template: tpl,
	}
}

func (s *EmailStrategy) Write(p []byte) (n int, err error) {
	s.sender.SendAsync(mailSender.Message{
		Message:  s.Message,
		Template: s.Template,
		Data:     mailSender.EmailData{"Data": string(p)},
	})
	return len(p), nil
}

// Create creates an instance of the logger
func Create(config *Config) Logged {
	for _, logger := range config.Loggers {
		go logger.reader()
	}
	return &Log{config: config}
}

// Default created default logger. Writes to stdout and stderr
func Default(chanBuffer uint) Logged {
	config := &Config{
		TimeFormat: time.RFC3339Nano,
		Loggers: LoggerMap{
			LoggerInfo: &Logger{
				Channel: make(chan string, chanBuffer),
				Strategies: []io.Writer{
					GetFileStrategy(os.Stdout.Name()),
				},
			},
			LoggerWrn: &Logger{
				Channel: make(chan string, chanBuffer),
				Strategies: []io.Writer{
					GetFileStrategy(os.Stdout.Name()),
				},
			},
			LoggerErr: &Logger{
				Channel: make(chan string, chanBuffer),
				Strategies: []io.Writer{
					GetFileStrategy(os.Stderr.Name()),
				},
			},
		},
	}
	for _, logger := range config.Loggers {
		go logger.reader()
	}
	return &Log{config: config}
}

func (l *Logger) reader() {
	for msg := range l.Channel {
		l.writeMessage(msg)
	}
}

func (l *Logger) writeMessage(msg string) {
	for _, strategy := range l.Strategies {
		if n, err := strategy.Write([]byte(msg)); err != nil {
			log.Println(fmt.Sprintf("%d characters have been written. %s", n, err.Error()))
		}
	}
}

func printNotConfiguredMessage(code uint, skip int) {
	if _, fileName, fileLine, ok := runtime.Caller(skip); ok {
		log.Println(fmt.Sprintf("%s:%d Logger %s not configured", fileName, fileLine, LoggerName(code)))
		return
	}
	log.Println(fmt.Sprintf("Logger %s not configured", LoggerName(code)))
}

// GetLoggerInterfaceByType returns io.Writer interface for logging in third-party libraries
func (a *Log) GetLoggerInterfaceByType(loggerType uint) io.Writer {
	if logger := a.config.Loggers[loggerType]; logger != nil {
		return logger
	}
	printNotConfiguredMessage(loggerType, 2)
	return &DefaultStrategy{}
}

// Info method for recording informational messages
func (a *Log) Info(msg string) *Log {
	if logger := a.config.Loggers[LoggerInfo]; logger != nil {
		logger.Channel <- a.prepareLog(time.Now(), msg, 2)
	} else {
		printNotConfiguredMessage(LoggerInfo, 2)
	}
	return a
}

// Infof method of recording formatted informational messages
func (a *Log) Infof(format string, p ...interface{}) *Log {
	if logger := a.config.Loggers[LoggerInfo]; logger != nil {
		logger.Channel <- a.prepareLog(time.Now(), fmt.Sprintf(format, p...), 2)
	} else {
		printNotConfiguredMessage(LoggerInfo, 2)
	}
	return a
}

// Warning method for recording warning messages
func (a *Log) Warning(msg string) *Log {
	if a.config.Loggers[LoggerWrn] != nil {
		a.config.Loggers[LoggerWrn].Channel <- a.prepareLog(time.Now(), msg, 2)
	} else {
		printNotConfiguredMessage(LoggerWrn, 2)
	}
	return a
}

// Method for recording errors without stack
func (a *Log) Error(err error) *Log {
	if a.config.Loggers[LoggerErr] != nil {
		if err != nil {
			a.config.Loggers[LoggerErr].Channel <- a.prepareLog(time.Now(), err.Error(), 2)
		}
	} else {
		printNotConfiguredMessage(LoggerErr, 2)
	}
	return a
}

// ErrorDebug method for recording errors with stack
func (a *Log) ErrorDebug(err error) *Log {
	if a.config.Loggers[LoggerErr] != nil {
		if err != nil {
			msg := fmt.Sprintf(messageFormatErrorDebug, a.prepareLog(time.Now(), err.Error(), 2), string(debug.Stack()))
			a.config.Loggers[LoggerErr].Channel <- msg
		}
	} else {
		printNotConfiguredMessage(LoggerErr, 2)
	}
	return a
}

func (a *Log) getTimeFormat() string {
	if format := a.config.TimeFormat; format != "" {
		return format
	}
	return time.RFC3339Nano
}

func (a *Log) prepareLog(time time.Time, msg string, skip int) string {
	if _, fileName, fileLine, ok := runtime.Caller(skip); ok && a.config.IgnoreFileLine {
		return fmt.Sprintf(
			messageFormatWithFileLine,
			time.Format(a.getTimeFormat()),
			fileName,
			fileLine,
			msg,
		)
	}
	return fmt.Sprintf(
		messageFormatDefault,
		time.Format(a.getTimeFormat()),
		msg,
	)
}

func openFile(filePath string) (afero.File, error) {
	if filePath == "" {
		log.Println(afero.ErrFileNotFound)
		return nil, afero.ErrFileNotFound
	}
	return fs.OpenFile(filePath, fileOptions, filePermission)
}

func createDirectoryIfNotExist(dirPath string) error {
	_, err := fs.Stat(dirPath)
	if os.IsNotExist(err) {
		return fs.MkdirAll(dirPath, filePermission)
	}
	return err
}

func addDirectory(filePath string) error {
	if filePath == "" {
		return errCanNotCreateDirectory
	}
	dir, _ := filepath.Split(filePath)
	return createDirectoryIfNotExist(dir)
}
