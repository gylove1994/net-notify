package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
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
		notifyBackend:  notify.BackendNotifySend,
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
	case "groups":
		groupsCmd(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`net-notify — 网络连通性探测，失败时通过 notify-send（默认）或 dms 发出通知

用法:
  net-notify run [flags]            持续探测（默认每分钟）
  net-notify check [flags]          单次探测，仅设置退出码（不发送通知）
  net-notify test-notify [flags]    发送一条测试通知，验证 notify-send / dms 是否可用
  net-notify groups list|set-name   列出或编辑配置文件中的分组名称（需 -config）

常用 flags:
  -config string      JSON 配置文件路径
  -interval duration  探测周期间隔（默认 1m，仅 run）
  -timeout duration   单次 HTTP 请求超时（默认 10s）
  -url string         目标 URL（可重复；无 -url 且无配置时为内置 Google+GitHub + 百度 分组；-url 出现时忽略配置的 urls/groups）
  -notify-when        与 -url 联用：flat 一组 URL 的策略 any-fail | all-fail（默认 any-fail）
  -once               只运行一轮后退出（仍会在失败时通知，但不使用冷却；仅 run）
  -alert-cooldown     持续失败时的重复通知最小间隔（默认 15m，仅 run 且非 -once）
  -notify-backend     notify-send | dms（默认 notify-send）
  -notify-urgency    严重程度：low | normal | critical（默认 critical；Freedesktop 低/普通/紧急）
                      说明：选用 dms 且为 normal 时走 dms notify；否则走 notify-send -u（由桌面通知服务显示）
  -notify-timeout-ms  通知超时（毫秒；notify-send -t）
  -notify-icon        notify-send -i / dms --icon
  -notify-app         notify-send -a / dms --app
  -dms-path           选用 dms 后端时 dms 可执行文件路径（默认 PATH 中 dms）
  -verbose            每轮探测后向 stderr 打印一行摘要（便于 journalctl 观察周期）

test-notify 额外 flags:
  -summary string      自定义通知标题（默认内置测试标题）
  -body string          自定义通知正文（默认带时间戳的测试正文）
  -notify-urgency       同 run

groups 子命令:
  net-notify groups list -config <path>
      打印分组表：索引(从0)、json 中的 name、生效名称、URL 数量、notify_when；仅适用含 \"groups\" 的配置
  net-notify groups set-name <index> <name> -config <path>
      将 groups[index].name 写入文件；<name> 可含空格（置于命令行末尾）；写前会校验整份配置

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
	fs.StringVar(&o.notifyBackend, "notify-backend", o.notifyBackend, "notify-send (default) or dms")
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
	fs.StringVar(&o.notifyBackend, "notify-backend", o.notifyBackend, "notify-send (default) or dms")
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

func groupsCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: net-notify groups list|set-name ... (-h for help)")
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		groupsListCmd(args[1:])
	case "set-name":
		groupsSetNameCmd(args[1:])
	case "help", "-h", "--help":
		fmt.Print(`groups 子命令（须指定 -config）:
  net-notify groups list -config <path>
  net-notify groups set-name <index> <name> -config <path>
  索引 index 从 0 起，与 JSON 中 groups 数组下标一致。
`)
	default:
		fmt.Fprintf(os.Stderr, "unknown groups subcommand: %s\n", args[0])
		os.Exit(2)
	}
}

func groupsListCmd(args []string) {
	fs := flag.NewFlagSet("groups list", flag.ContinueOnError)
	var cfg string
	fs.StringVar(&cfg, "config", "", "JSON config path")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if cfg == "" {
		fmt.Fprintln(os.Stderr, "groups list: -config is required")
		os.Exit(2)
	}
	f, err := config.Load(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(f.Groups) == 0 {
		fmt.Println("no groups in config (flat \"urls\" only or empty groups); nothing to list")
		return
	}
	fmt.Println("index\tname_in_json\teffective_name\turls\tnotify_when")
	for i, g := range f.Groups {
		jsonName := "(empty)"
		if strings.TrimSpace(g.Name) != "" {
			jsonName = g.Name
		}
		fmt.Printf("%d\t%s\t%s\t%d\t%s\n", i, jsonName, g.EffectiveName(i), len(g.URLs), g.NotifyWhen)
	}
}

func groupsSetNameCmd(args []string) {
	fs := flag.NewFlagSet("groups set-name", flag.ContinueOnError)
	var cfg string
	fs.StringVar(&cfg, "config", "", "JSON config path")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if cfg == "" {
		fmt.Fprintln(os.Stderr, "groups set-name: -config is required")
		os.Exit(2)
	}
	rest := fs.Args()
	if len(rest) < 2 {
		fmt.Fprintln(os.Stderr, "groups set-name: need <index> and <name>")
		os.Exit(2)
	}
	idx, err := strconv.Atoi(rest[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "groups set-name: invalid index %q: %v\n", rest[0], err)
		os.Exit(2)
	}
	name := strings.Join(rest[1:], " ")
	if err := config.UpdateGroupName(cfg, idx, name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("groups set-name: ok")
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
	groupCD := make(map[string]*state.Cooldown)
	cooldownFor := func(name string) *state.Cooldown {
		cd, ok := groupCD[name]
		if !ok {
			cd = &state.Cooldown{Cooldown: o.alertCooldown}
			groupCD[name] = cd
		}
		return cd
	}
	n := buildNotifier(o.notifyBackend, o.notifyUrgency, o.dmsPath, o.notifyApp, o.notifyTimeout, o.notifyIcon)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runRound := func() bool {
		results := probe.ProbeAll(ctx, layout.FlatURLs, o.requestTimeout)
		outcomes, alerting := policy.EvaluateGroups(layout, results)
		var triggered []string
		for _, oc := range outcomes {
			if oc.Fires {
				triggered = append(triggered, oc.Name)
			}
		}
		if o.verbose {
			ok := 0
			for _, r := range results {
				if r.OK() {
					ok++
				}
			}
			fmt.Fprintf(os.Stderr, "%s net-notify: probe round urls=%d ok=%d failing=%v triggered_groups=%v\n",
				time.Now().Format(time.RFC3339), len(results), ok, alerting, triggered)
		}
		now := time.Now()
		if !alerting {
			for _, oc := range outcomes {
				cooldownFor(oc.Name).ShouldNotify(now, false)
			}
			return false
		}
		for _, oc := range outcomes {
			cd := cooldownFor(oc.Name)
			if !oc.Fires {
				cd.ShouldNotify(now, false)
				continue
			}
			should := o.once || cd.ShouldNotify(now, true)
			if !should {
				continue
			}
			body := probe.FormatReport(oc.Results)
			if oc.Name != "default" {
				body = "组名称: " + oc.Name + "\n\n" + body
			}
			body = notify.TruncateBody(body, 8000)
			// DMS over DBus rejects some longer CJK summaries (bogus "not-utf8" error); keep title short.
			summaryTitle := "网络探测失败：" + oc.Name
			if oc.Name == "default" {
				summaryTitle = "网络探测失败"
			}
			summary := notify.TruncateSummary(summaryTitle, 32)
			nctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := n.Notify(nctx, summary, body)
			cancel()
			if err != nil {
				fmt.Fprintf(os.Stderr, "notify: %v\n", err)
			}
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
