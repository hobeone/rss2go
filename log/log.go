package log

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
)

func init() {
	fmter := &logrus.TextFormatter{}
	fmter.FullTimestamp = true
	fmter.TimestampFormat = time.StampMilli
	fmter.TimestampFormat = "Mon, 02 Jan 2006 15:04:05.000 -0700"
	logrus.SetFormatter(fmter)

	// Output to stderr instead of stdout, could also be a file.
	logrus.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	logrus.SetLevel(logrus.WarnLevel)
}

// SetNullOutput sets the looger to send everything to /dev/null.
// useful when running unittests.
func SetNullOutput() {
	logrus.SetOutput(ioutil.Discard)
}
