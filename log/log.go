package log

import (
	"io/ioutil"
	"os"

	"github.com/Sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

func SetupLogger() {
	fmter := &prefixed.TextFormatter{}
	logrus.SetFormatter(fmter)
	logrus.SetOutput(os.Stdout)
	// Only log the info severity or above.
	logrus.SetLevel(logrus.InfoLevel)
}

// SetNullOutput sets the looger to send everything to /dev/null.
// useful when running unittests.
func SetNullOutput() {
	logrus.SetOutput(ioutil.Discard)
}
