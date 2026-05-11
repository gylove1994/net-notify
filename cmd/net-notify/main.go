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
	"github.com/gylove1994/net-notify/internal/policy"
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
	configPath string
	interval   time.Duration
	urls       stringList

	urlFromCLI        bool
	useConfigGroups   bool
	configProbeLayout policy.Layout
	notifyWhen        string // any_fail | all_fail for flat URL mode (CLI or default URLs)

	requestTimeout time.Duration
	once           bool
	alertCooldown  time.Duration
	notifyBackend  string
	notifyUrgency  string
	notifyTimeout  int
	notifyIcon     string
	notifyApp      string
	dmsPath        string
	verbose        bool
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
	case "test-notify":
		testNotifyCmd(os.Args[2:])
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
  net-notify run [flags]            持续探测（默认每分钟）
  net-notify check [flags]          单次探测，仅设置退出码（不发送通知）
  net-notify test-notify [flags]    发送一条测试通知，验证 dms / notify-send 是否可用

常用 flags:
  -config string      JSON 配置文件路径
  -interval duration  探测周期间隔（默认 1m，仅 run）
  -timeout duration   单次 HTTP 请求超时（默认 10s）
  -url string         目标 URL（可重复；无 -url 且无配置时为内置 Google+GitHub + 百度 分组；-url 出现时忽略配置的 urls/groups）
  -notify-when        与 -url 联用：flat 一组 URL 的策略 any-fail | all-fail（默认 any-fail）
  -once               只运行一轮后退出（仍会在失败时通知，但不使用冷却；仅 run）
  -alert-cooldown     持续失败时的重复通知最小间隔（默认 15m，仅 run 且非 -once）
  -notify-backend     dms | notify-send（默认 dms）
  -notify-urgency    严重程度：low | normal | critical（默认 critical；与 DMS 设置里的低/普通/紧急对应）
                      说明：dms notify 子命令无 urgency 参数；非 normal 时使用 notify-send -u（仍由 DMS 通知服务接收）
  -notify-timeout-ms  dms notify --timeout（毫秒）
  -notify-icon        dms notify --icon
  -notify-app         dms notify --app
  -dms-path           dms 可执行文件路径（默认 PATH 中 dms）
  -verbose            每轮探测后向 stderr 打印一行摘要（便于 journalctl 观察周期）

test-notify 额外 flags:
  -summary string      自定义通知标题（默认内置测试标题）
  -body string          自定义通知正文（默认带时间戳的测试正文）
  -notify-urgency       同 run

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
	if len(f.Groups) > 0 {
		layout, err := config.LayoutFromGroups(f.Groups)
		if err != nil {
			return base, fmt.Errorf("config: %w", err)
		}
		base.useConfigGroups = true
		base.configProbeLayout = layout
	} else if len(f.URLs) > 0 {
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
	if f.NotifyUrgency != "" {
		base.notifyUrgency = f.NotifyUrgency
	}
	if f.Verbose {
		base.verbose = true
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
	fs.StringVar(&o.notifyWhen, "notify-when", o.notifyWhen, "any-fail or all-fail (flat -url list only)")
	fs.BoolVar(&o.once, "once", false, "single round then exit")
	fs.DurationVar(&o.alertCooldown, "alert-cooldown", o.alertCooldown, "min time between repeated failure alerts")
	fs.StringVar(&o.notifyBackend, "notify-backend", o.notifyBackend, "dms or notify-send")
	fs.StringVar(&o.notifyUrgency, "notify-urgency", o.notifyUrgency, "low, normal, or critical")
	fs.IntVar(&o.notifyTimeout, "notify-timeout-ms", o.notifyTimeout, "dms notify --timeout (ms)")
	fs.StringVar(&o.notifyIcon, "notify-icon", o.notifyIcon, "dms notify --icon")
	fs.StringVar(&o.notifyApp, "notify-app", o.notifyApp, "dms notify --app")
	fs.StringVar(&o.dmsPath, "dms-path", o.dmsPath, "path to dms binary")
	fs.BoolVar(&o.verbose, "verbose", o.verbose, "log each probe round to stderr")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if len(cliURLs) > 0 {
		o.urls = cliURLs
		o.urlFromCLI = true
		o.useConfigGroups = false
	}
	return o, nil
}

func parseTestNotifyFlags(args []string, base runOptions) (runOptions, string, string, error) {
	fs := flag.NewFlagSet("test-notify", flag.ContinueOnError)
	o := base
	var summaryFlag, bodyFlag string
	fs.StringVar(&o.configPath, "config", "", "JSON config path")
	fs.StringVar(&o.notifyBackend, "notify-backend", o.notifyBackend, "dms or notify-send")
	fs.IntVar(&o.notifyTimeout, "notify-timeout-ms", o.notifyTimeout, "dms notify --timeout (ms)")
	fs.StringVar(&o.notifyIcon, "notify-icon", o.notifyIcon, "dms notify --icon")
	fs.StringVar(&o.notifyApp, "notify-app", o.notifyApp, "dms notify --app")
	fs.StringVar(&o.dmsPath, "dms-path", o.dmsPath, "path to dms binary")
	fs.StringVar(&o.notifyUrgency, "notify-urgency", o.notifyUrgency, "low, normal, or critical")
	fs.StringVar(&summaryFlag, "summary", "", "notification summary (default if empty)")
	fs.StringVar(&bodyFlag, "body", "", "notification body (default if empty)")
	if err := fs.Parse(args); err != nil {
		return o, "", "", err
	}
	summary := summaryFlag
	if summary == "" {
		summary = "net-notify 测试通知"
	}
	body := bodyFlag
	if body == "" {
		body = fmt.Sprintf("这是一条用于验证通知链路的测试消息。\n时间: %s", time.Now().Format(time.RFC3339))
	}
	return o, summary, body, nil
}

func testNotifyCmd(args []string) {
	base, err := mergeConfig(defaults(), configPathFromArgs(args))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	o, summary, body, err := parseTestNotifyFlags(args, base)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	n := buildNotifier(o.notifyBackend, o.notifyUrgency, o.dmsPath, o.notifyApp, o.notifyTimeout, o.notifyIcon)
	summary = notify.TruncateSummary(summary, 120)
	body = notify.TruncateBody(body, 8000)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.Notify(ctx, summary, body); err != nil {
		fmt.Fprintf(os.Stderr, "test-notify: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("test-notify: ok")
}

func effectiveURLs(urls []string) []string {
	if len(urls) == 0 {
		out := make([]string, len(probe.DefaultURLs))
		copy(out, probe.DefaultURLs)
		return out
	}
	return append([]string(nil), urls...)
}

func resolveProbeLayout(o runOptions) (policy.Layout, error) {
	if o.urlFromCLI {
		when, err := policy.ParseWhen(o.notifyWhen)
		if err != nil {
			return policy.Layout{}, fmt.Errorf("-notify-when: %w", err)
		}
		return policy.SingleGroup(effectiveURLs(o.urls), when), nil
	}
	if strings.TrimSpace(o.notifyWhen) != "" {
		return policy.Layout{}, fmt.Errorf("-notify-when is only valid with -url")
	}
	if o.useConfigGroups {
		return o.configProbeLayout, nil
	}
	if len(o.urls) > 0 {
		return policy.SingleGroup(effectiveURLs(o.urls), policy.WhenAnyFail), nil
	}
	return policy.BuiltinDefaultLayout(), nil
}

func buildNotifier(backend, urgency, dmsPath, app string, timeoutMs int, icon string) notify.Notifier {
	u := notify.NormalizeUrgency(urgency)
	switch notify.BackendName(backend) {
	case notify.BackendNotifySend:
		return &notify.NotifySend{Urgency: u, App: app, Icon: icon, TimeoutMs: timeoutMs}
	default:
		// dms subcommand cannot set urgency; use notify-send for non-normal so DMS NotificationServer still gets hints.
		if u == notify.UrgencyNormal {
			return &notify.DMS{Path: dmsPath, App: app, TimeoutMs: timeoutMs, Icon: icon}
		}
		return &notify.NotifySend{Urgency: u, App: app, Icon: icon, TimeoutMs: timeoutMs}
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
	layout, err := resolveProbeLayout(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	cd := &state.Cooldown{Cooldown: o.alertCooldown}
	n := buildNotifier(o.notifyBackend, o.notifyUrgency, o.dmsPath, o.notifyApp, o.notifyTimeout, o.notifyIcon)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runRound := func() bool {
		results := probe.ProbeAll(ctx, layout.FlatURLs, o.requestTimeout)
		failing, triggered := policy.Evaluate(layout, results)
		if o.verbose {
			ok := 0
			for _, r := range results {
				if r.OK() {
					ok++
				}
			}
			fmt.Fprintf(os.Stderr, "%s net-notify: probe round urls=%d ok=%d failing=%v triggered_groups=%v\n",
				time.Now().Format(time.RFC3339), len(results), ok, failing, triggered)
		}
		if !failing {
			cd.ShouldNotify(time.Now(), false)
			return false
		}
		should := o.once || cd.ShouldNotify(time.Now(), true)
		if !should {
			return true
		}
		body := probe.FormatReport(results)
		if len(triggered) > 0 {
			body = "触发分组: " + strings.Join(triggered, ", ") + "\n\n" + body
		}
		body = notify.TruncateBody(body, 8000)
		// DMS over DBus rejects some longer CJK summaries (bogus "not-utf8" error); keep title short.
		summary := notify.TruncateSummary("网络探测失败", 32)
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
	layout, err := resolveProbeLayout(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx := context.Background()
	results := probe.ProbeAll(ctx, layout.FlatURLs, o.requestTimeout)
	failing, triggered := policy.Evaluate(layout, results)
	report := probe.FormatReport(results)
	if len(triggered) > 0 {
		report = "触发分组: " + strings.Join(triggered, ", ") + "\n\n" + report
	}
	fmt.Println(report)
	if failing {
		os.Exit(1)
	}
}
