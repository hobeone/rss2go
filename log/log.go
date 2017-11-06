package log

import (
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

func SetupLogger(logger *logrus.Logger) {
	fmter := &prefixed.TextFormatter{}
	logger.Formatter = fmter
	logger.Out = os.Stdout
	// Only log the info severity or above.
	logger.Level = logrus.InfoLevel
}

// SetNullOutput sets the looger to send everything to /dev/null.
// useful when running unittests.
func SetNullOutput() {
	logrus.SetOutput(ioutil.Discard)
}
