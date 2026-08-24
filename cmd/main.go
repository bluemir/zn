package cmd

import (
	"context"
	"fmt"
	"io"
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
	describe        = `Zn Editor. Personalized vim clone editor`
	defaultLogLevel = logrus.WarnLevel
)

func Run() error {
	conf := struct {
		logLevel  int
		logFormat string
		logFile   string // 비면 로그를 버린다. 편집기가 화면을 차지해서 낼 자리가 없다.

		files []string // 편집할 file 들. tab 으로 열린다. 없으면 tab 없이 시작한다(ADR-0064).
	}{}

	app := kingpin.New(buildinfo.AppName, describe)
	app.Version(buildinfo.Describe())

	app.Flag("verbose", "Log level").
		Short('v').
		CounterVar(&conf.logLevel)
	app.Flag("log-format", "Log format").
		StringVar(&conf.logFormat)
	app.Flag("log-file", "Log file. 편집기가 화면을 차지하므로 이것 없이는 로그가 남지 않는다").
		StringVar(&conf.logFile)
	app.Arg("files", "files").
		StringsVar(&conf.files)

	app.PreAction(func(*kingpin.ParseContext) error {
		level := logrus.Level(conf.logLevel) + defaultLogLevel

		// **적을 곳이 없으면 버린다. stderr 로 흘리지 않는다.**
		//
		// 편집기는 대체 화면(alt screen) 으로 터미널을 통째로 차지한다. 그 터미널이 곧
		// stderr 라, 도는 동안 로그 한 줄이 나가면 화면 한가운데에 찍혀 그림이 깨진다.
		// 끄는 것이 기본값(WarnLevel) 이라 여태 드러나지 않았을 뿐이다.
		//
		// 파일을 받는 플래그는 처음부터 있었고 「지정 하지 않으면 log 가 남지 않는다」는
		// 것이 그때 적어 둔 뜻이다. 그 뜻대로 잇는다.
		out, err := openLogFile(conf.logFile)
		if err != nil {
			return err
		}

		logrus.SetOutput(out)
		logrus.SetLevel(level)
		logrus.SetReportCaller(true)

		// **맨 먼저 무엇으로 지은 binary 인지 찍는다.**
		//
		// 로그를 받아 보는 쪽이 가장 먼저 물어야 할 것이 이것이다. 고친 것이 들어 있는
		// binary 로 잡은 로그인지 아닌지를 로그 안에서 가릴 수 없으면, 멀쩡한 로그를 놓고
		// 없는 버그를 찾게 된다 — 실제로 겪었다.
		//
		// `:version` 과 `--version` 이 쓰는 그 줄 그대로다(buildinfo.Describe).
		logrus.Info(buildinfo.Describe())
		logrus.Infof("logrus level: %s, log file: %s", level, conf.logFile)

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

// openLogFile 은 로그를 적을 곳이다. 경로가 비면 io.Discard 로 버린다.
//
// 파일은 이어 쓴다. 한글 입력기가 얽힌 버그처럼 여러 번 재현해서 견주는 일이 있어서,
// 띄울 때마다 앞의 기록을 지우면 방금 잡은 것을 잃는다.
func openLogFile(path string) (io.Writer, error) {
	if path == "" {
		return io.Discard, nil
	}

	// 닫지 않는다. 프로세스가 끝날 때까지 쓰는 것이고, 끝나면 OS 가 닫는다.
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, errors.Wrapf(err, "로그 파일을 열 수 없다: %s", path)
	}

	return file, nil
}
