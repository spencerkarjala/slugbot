package slog

import (
	"log"
	"os"
)

// Level constants
const (
	LevelTrace = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel = LevelDebug

func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func SetLevel(lvl int) {
	currentLevel = lvl
}

func trace(v ...interface{}) {
	if LevelTrace >= currentLevel {
		log.SetPrefix("TRACE: ")
		log.Println(v...)
	}
}
func debug(v ...interface{}) {
	if LevelDebug >= currentLevel {
		log.SetPrefix("DEBUG: ")
		log.Println(v...)
	}
}
func info(v ...interface{}) {
	if LevelInfo >= currentLevel {
		log.SetPrefix("INFO:  ")
		log.Println(v...)
	}
}
func warn(v ...interface{}) {
	if LevelWarn >= currentLevel {
		log.SetPrefix("WARN:  ")
		log.Println(v...)
	}
}
func errorLog(v ...interface{}) {
	if LevelError >= currentLevel {
		log.SetPrefix("ERROR: ")
		log.Println(v...)
	}
}
func fatal(v ...interface{}) { log.SetPrefix("FATAL: "); log.Fatal(v...) }

// Public API
var (
	Trace = trace
	Debug = debug
	Info  = info
	Warn  = warn
	Error = errorLog
	Fatal = fatal
)
