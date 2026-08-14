package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/alecthomas/kingpin/v2"
	"github.com/cockroachdb/errors"
	"github.com/sirupsen/logrus"

	"github.com/bluemir/zn/internal/buildinfo"
	"github.com/bluemir/zn/internal/core"
)

const (
	describe        = ``
	defaultLogLevel = logrus.WarnLevel
)

func Run() error {
	conf := struct {
		logLevel  int
		logFormat string
		logFile   string // TODO 지정 하지 않으면 log 가 남지 않는다.

		files []string // 편집할 file 들. tab 으로 열린다.
	}{}

	app := kingpin.New(buildinfo.AppName, describe)
	app.Version(buildinfo.Version + "\nbuildtime:" + buildinfo.BuildTime)

	app.Flag("verbose", "Log level").
		Short('v').
		CounterVar(&conf.logLevel)
	app.Flag("log-format", "Log format").
		StringVar(&conf.logFormat)
	app.Flag("log-file", "Log file").
		StringVar(&conf.logFile)
	app.Arg("files", "files").
		StringsVar(&conf.files)

	app.PreAction(func(*kingpin.ParseContext) error {
		level := logrus.Level(conf.logLevel) + defaultLogLevel
		logrus.SetOutput(os.Stderr)
		logrus.SetLevel(level)
		logrus.SetReportCaller(true)
		logrus.Infof("logrus level: %s", level)

		callerPrettyfier := func(f *runtime.Frame) (string, string) {
			/* https://github.com/sirupsen/logrus/issues/63#issuecomment-476486166 */
			return "", fmt.Sprintf("%s:%d", f.File, f.Line)
		}

		switch conf.logFormat {
		case "text-color":
			logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true, CallerPrettyfier: callerPrettyfier})
		case "json":
			logrus.SetFormatter(&logrus.JSONFormatter{CallerPrettyfier: callerPrettyfier})
		case "", "text":
			logrus.StandardLogger().Formatter = &logrus.TextFormatter{CallerPrettyfier: callerPrettyfier}
		default:
			return errors.Errorf("unknown log format")
		}

		return nil
	})

	_, err := app.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGTERM,
		syscall.SIGINT,
	)
	defer stop()

	return core.Run(ctx, conf.files)
}
