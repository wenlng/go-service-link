package helper

// OutputLogType .
type OutputLogType string

const (
	OutputLogTypeWarn  OutputLogType = "warn"
	OutputLogTypeInfo                = "info"
	OutputLogTypeError               = "error"
	OutputLogTypeDebug               = "debug"
)

// OutputLogCallback ..
type OutputLogCallback = func(logType OutputLogType, message string)
