package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gylove1994/net-notify/internal/config"
	"github.com/gylove1994/net-notify/internal/notify"
	"github.com/gylove1994/net-notify/internal/probe"
	"github.com/gylove1994/net-notify/internal/state"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type runOptions struct {
	configPath     string
	interval       time.Duration
	requestTimeout time.Duration
	urls           stringList
	once           bool
	alertCooldown  time.Duration
	notifyBackend  string
	notifyTimeout  int
	notifyIcon     string
	notifyApp      string
	dmsPath        string
}

func defaults() runOptions {
	return runOptions{
		interval:       time.Minute,
		requestTimeout: 10 * time.Second,
		alertCooldown:  15 * time.Minute,
		notifyBackend:  notify.BackendDMS,
		notifyTimeout:  30000,
		notifyIcon:     "network-error",
		notifyApp:      "net-notify",
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "check":
		checkCmd(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`net-notify — 网络连通性探测，失败时通过 dms / notify-send 发出通知

用法:
  net-notify run [flags]    持续探测（默认每分钟）
  net-notify check [flags]  单次探测，仅设置退出码（不发送通知）

常用 flags:
  -config string      JSON 配置文件路径
  -interval duration  探测周期间隔（默认 1m，仅 run）
  -timeout duration   单次 HTTP 请求超时（默认 10s）
  -url string         目标 URL（可重复；默认三大站点）
  -once               只运行一轮后退出（仍会在失败时通知，但不使用冷却；仅 run）
  -alert-cooldown     持续失败时的重复通知最小间隔（默认 15m，仅 run 且非 -once）
  -notify-backend     dms | notify-send（默认 dms）
  -notify-timeout-ms  dms notify --timeout（毫秒）
  -notify-icon        dms notify --icon
  -notify-app         dms notify --app
  -dms-path           dms 可执行文件路径（默认 PATH 中 dms）

`)
}

func mergeConfig(base runOptions, path string) (runOptions, error) {
	if path == "" {
		return base, nil
	}
	f, err := config.Load(path)
	if err != nil {
		return base, err
	}
	if f.Interval != "" {
		d, err := time.ParseDuration(f.Interval)
		if err != nil {
			return base, fmt.Errorf("config interval: %w", err)
		}
		base.interval = d
	}
	if f.Timeout != "" {
		d, err := time.ParseDuration(f.Timeout)
		if err != nil {
			return base, fmt.Errorf("config timeout: %w", err)
		}
		base.requestTimeout = d
	}
	if len(f.URLs) > 0 {
		base.urls = append(stringList(nil), f.URLs...)
	}
	if f.AlertCooldown != "" {
		d, err := time.ParseDuration(f.AlertCooldown)
		if err != nil {
			return base, fmt.Errorf("config alert_cooldown: %w", err)
		}
		base.alertCooldown = d
	}
	if f.NotifyTimeoutMs > 0 {
		base.notifyTimeout = f.NotifyTimeoutMs
	}
	if f.NotifyIcon != "" {
		base.notifyIcon = f.NotifyIcon
	}
	if f.NotifyApp != "" {
		base.notifyApp = f.NotifyApp
	}
	if f.NotifyBackend != "" {
		base.notifyBackend = f.NotifyBackend
	}
	if f.DMSPath != "" {
		base.dmsPath = f.DMSPath
	}
	return base, nil
}

func configPathFromArgs(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-config" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], "-config=") {
			return strings.TrimPrefix(args[i], "-config=")
		}
	}
	return ""
}

func parseRunFlags(args []string, base runOptions) (runOptions, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	o := base
	var cliURLs stringList
	fs.StringVar(&o.configPath, "config", "", "JSON config path")
	fs.DurationVar(&o.interval, "interval", o.interval, "poll interval")
	fs.DurationVar(&o.requestTimeout, "timeout", o.requestTimeout, "per-request HTTP timeout")
	fs.Var(&cliURLs, "url", "probe URL (repeatable)")
	fs.BoolVar(&o.once, "once", false, "single round then exit")
	fs.DurationVar(&o.alertCooldown, "alert-cooldown", o.alertCooldown, "min time between repeated failure alerts")
	fs.StringVar(&o.notifyBackend, "notify-backend", o.notifyBackend, "dms or notify-send")
	fs.IntVar(&o.notifyTimeout, "notify-timeout-ms", o.notifyTimeout, "dms notify --timeout (ms)")
	fs.StringVar(&o.notifyIcon, "notify-icon", o.notifyIcon, "dms notify --icon")
	fs.StringVar(&o.notifyApp, "notify-app", o.notifyApp, "dms notify --app")
	fs.StringVar(&o.dmsPath, "dms-path", o.dmsPath, "path to dms binary")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if len(cliURLs) > 0 {
		o.urls = cliURLs
	}
	return o, nil
}

func effectiveURLs(urls []string) []string {
	if len(urls) == 0 {
		out := make([]string, len(probe.DefaultURLs))
		copy(out, probe.DefaultURLs)
		return out
	}
	return append([]string(nil), urls...)
}

func buildNotifier(backend string, dmsPath, app string, timeoutMs int, icon string) notify.Notifier {
	switch notify.BackendName(backend) {
	case notify.BackendNotifySend:
		return notify.NotifySend{}
	default:
		return &notify.DMS{Path: dmsPath, App: app, TimeoutMs: timeoutMs, Icon: icon}
	}
}

func runCmd(args []string) {
	base, err := mergeConfig(defaults(), configPathFromArgs(args))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	o, err := parseRunFlags(args, base)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	urls := effectiveURLs(o.urls)
	cd := &state.Cooldown{Cooldown: o.alertCooldown}
	n := buildNotifier(o.notifyBackend, o.dmsPath, o.notifyApp, o.notifyTimeout, o.notifyIcon)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runRound := func() bool {
		results := probe.ProbeAll(ctx, urls, o.requestTimeout)
		failing := probe.AnyFail(results)
		if !failing {
			cd.ShouldNotify(time.Now(), false)
			return false
		}
		should := o.once || cd.ShouldNotify(time.Now(), true)
		if !should {
			return true
		}
		body := notify.TruncateBody(probe.FormatReport(results), 8000)
		summary := notify.TruncateSummary("网络探测失败（任一目标异常）", 120)
		nctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := n.Notify(nctx, summary, body)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "notify: %v\n", err)
		}
		return true
	}

	if o.once {
		failing := runRound()
		if failing {
			os.Exit(1)
		}
		return
	}

	t := time.NewTicker(o.interval)
	defer t.Stop()
	// first round immediately
	_ = runRound()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = runRound()
		}
	}
}

func checkCmd(args []string) {
	base, err := mergeConfig(defaults(), configPathFromArgs(args))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	o, err := parseRunFlags(args, base)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	urls := effectiveURLs(o.urls)
	ctx := context.Background()
	results := probe.ProbeAll(ctx, urls, o.requestTimeout)
	if probe.AnyFail(results) {
		fmt.Println(probe.FormatReport(results))
		os.Exit(1)
	}
	fmt.Println(probe.FormatReport(results))
}
