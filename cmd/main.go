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

	// SIGINT 는 여기서 받지 않는다.
	//
	// 편집기가 도는 동안 터미널은 raw mode 라 `ctrl+c` 가 신호가 아니라 글자로 오고, 그것을
	// 종료 확인창으로 받는 것은 key 처리다. 신호로 오는 SIGINT 는 bubbletea 가 이미
	// 처리한다(InterruptMsg → 종료).
	//
	// 그리고 `:!` 로 셸 명령을 돌리는 동안에는 터미널이 cooked mode 로 돌아가서 `ctrl+c` 가
	// 진짜 SIGINT 가 된다. 그 신호는 도는 명령을 끊으라는 뜻이고, 편집기는 그때 신호를
	// 무시하도록 되어 있다. 여기서 root ctx 에 이어 두면 명령을 끊는 손짓 한 번에 편집기가
	// 통째로 끝나서 tab 구성과 undo 이력을 잃는다 (ADR-0045).
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGTERM,
	)
	defer stop()

	return core.Run(ctx, conf.files)
}
