package main

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danieljoos/wincred"
)

func testRoot(t *testing.T) string {
	t.Helper()
	base := filepath.Join(runtimeRoot, "verification", "local")
	if err := os.MkdirAll(base, 0700); err != nil { t.Fatal(err) }
	path, err := os.MkdirTemp(base, "本地验证-")
	if err != nil { t.Fatal(err) }
	return path
}

func TestConfiguration(t *testing.T) {
	old := configPath
	defer func(){ configPath = old }()
	root := testRoot(t)
	configPath = filepath.Join(root, "配置.toml")
	b, err := os.ReadFile("config.example.toml")
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(configPath, b, 0600); err != nil { t.Fatal(err) }
	c, err := loadConfig()
	if err != nil { t.Fatal(err) }
	if c.Monitor.Interval != 30 || !c.Monitor.Headless { t.Fatal("默认值错误") }
	if err := c.requireWatch(); err == nil { t.Fatal("空资料应当拒绝报名") }
	for _, interval := range []float64{0.000001, 0.75, 30, 97.123, 1e30} {
		if err := updateConfig([]string{"monitor", "poll_interval_seconds"}, interval); err != nil { t.Fatal(err) }
		loaded, err := loadConfig()
		if err != nil || loaded.Monitor.Interval != interval { t.Fatal("自定义间隔未保存") }
	}
	for _, value := range []float64{0,-1,math.Inf(1),math.NaN()} {
		c.Monitor.Interval = value
		if err := c.validate(); err == nil { t.Fatal("无效间隔未拒绝") }
	}
	if err := updateConfig([]string{"browser","kind"}, "chrome"); err != nil { t.Fatal(err) }
	if err := updateConfig([]string{"targets"}, []map[string]any{{"id":"local-config-one","name":"本地配置验证一","campus":"西校区"},{"id":"local-config-two","name":"本地配置验证二","campus":"高新校区"}}); err != nil { t.Fatal(err) }
	loaded, err := loadConfig()
	if err != nil || len(loaded.Targets)!=2 || loaded.Targets[0].ID!="local-config-one" { t.Fatal("优先顺序未保存") }
	written, err := os.ReadFile(configPath)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(written), "数组顺序就是优先顺序") { t.Fatal("配置注释丢失") }
	if err := os.WriteFile(configPath, []byte("unknown_field = 1\n"), 0600); err != nil { t.Fatal(err) }
	if _, err := loadConfig(); err == nil { t.Fatal("未知字段未拒绝") }
}

func TestJournalPersistenceAndAccount(t *testing.T) {
	root := testRoot(t)
	j, err := openJournal(root)
	if err != nil { t.Fatal(err) }
	target := Target{"local-persistence", "本地数据库持久保存验证", "西校区"}
	if err := j.record("local-test-account", target, "enter_pending"); err != nil { t.Fatal(err) }
	if err := j.db.Close(); err != nil { t.Fatal(err) }
	j, err = openJournal(root)
	if err != nil { t.Fatal(err) }
	defer j.db.Close()
	rows, err := j.pending("local-test-account")
	if err != nil || len(rows)!=1 || rows[0].Phase!="enter_pending" { t.Fatal("提交阶段未持久保存") }
	if _, err := j.pending("other-local-account"); err == nil { t.Fatal("错误账户未拒绝") }
	if strings.Contains(rows[0].Account,"local-test-account") || len(rows[0].Account)!=64 { t.Fatal("账户摘要无效") }
}

func TestSingleInstanceProcess(t *testing.T) {
	if root := os.Getenv("USTC_LOCK_TEST_ROOT"); root != "" {
		lock, err := lockInstance(root)
		if err != nil { os.Exit(23) }
		lock.Close()
		os.Exit(0)
	}
	root := testRoot(t)
	lock, err := lockInstance(root)
	if err != nil { t.Fatal(err) }
	child := func() error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSingleInstanceProcess$")
		cmd.Env = append(os.Environ(), "USTC_LOCK_TEST_ROOT="+root)
		return cmd.Run()
	}
	err = child()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode()!=23 { t.Fatal("第二进程未被阻止") }
	lock.Close()
	if err := child(); err != nil { t.Fatal("锁释放后无法启动",err) }
}

func TestCredentialManagerRoundTrip(t *testing.T) {
	username := "local-go-verification-" + time.Now().Format("20060102150405.000000000")
	password := "本地凭据编码验证-Aa123"
	if err := writePassword(username,password); err != nil { t.Fatal(err) }
	defer func(){ c,err:=wincred.GetGenericCredential(username+"@"+credentialService); if err!=nil { t.Error(err); return }; if err:=c.Delete();err!=nil {t.Error(err)} }()
	actual, err := readPassword(username)
	if err != nil || actual != password { t.Fatal("凭据编码或读取不一致") }
}

func TestFailFastData(t *testing.T) {
	for _, data := range []Object{{}, {"registrationNum":true}, {"registrationNum":json.Number("1.2")}} {
		if _, err := integer(data,"registrationNum"); err == nil { t.Fatal("无效人数未拒绝") }
	}
	if _,err:=parseTime("invalid");err==nil {t.Fatal("无效时间未拒绝")}
	if d:=seconds(0.00000000001);d<=0 {t.Fatal("正数间隔变成零")}
}

func TestInterruptibleWait(t *testing.T) {
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	start:=time.Now()
	if err:=sleep(ctx,time.Hour);err==nil || time.Since(start)>time.Second {t.Fatal("停止信号未结束等待")}
}
