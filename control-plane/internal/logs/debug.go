package logs

import "log"

func Debugf(component string, format string, args ...any) {
	log.Printf("[DEBUG] [%s] "+format, append([]any{component}, args...)...)
}

func Infof(component string, format string, args ...any) {
	log.Printf("[INFO] [%s] "+format, append([]any{component}, args...)...)
}

func Errorf(component string, format string, args ...any) {
	log.Printf("[ERROR] [%s] "+format, append([]any{component}, args...)...)
}
