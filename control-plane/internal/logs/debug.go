package logs

import "log"

func logf(level string, component string, format string, args ...any) {
	log.Printf("[%s] [%s] "+format, append([]any{level, component}, args...)...)
}

func Debugf(component string, format string, args ...any) {
	logf("DEBUG", component, format, args...)
}

func Infof(component string, format string, args ...any) {
	logf("INFO", component, format, args...)

}

func Warnf(component string, format string, args ...any) {
	logf("WARN", component, format, args...)
}

func Errorf(component string, format string, args ...any) {
	logf("ERROR", component, format, args...)
}
